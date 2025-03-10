/*
Copyright 2016 The Kubernetes Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package problemclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/golang/glog"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientset "k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/clock"

	"k8s.io/node-problem-detector/cmd/options"
	"k8s.io/node-problem-detector/pkg/version"
)

// Client is the interface of problem client
type Client interface {
	// GetConditions get all specific conditions of current node.
	GetConditions(ctx context.Context, conditionTypes []v1.NodeConditionType) ([]*v1.NodeCondition, error)
	// SetConditions set or update conditions of current node.
	SetConditions(ctx context.Context, conditionTypes []v1.NodeCondition) error
	// Eventf reports the event.
	Eventf(eventType string, source, reason, messageFmt string, args ...interface{})
	// GetNode returns the Node object of the node on which the
	// node-problem-detector runs.
	GetNode(ctx context.Context) (*v1.Node, error)
}

type nodeProblemClient struct {
	nodeName       string
	client         typedcorev1.CoreV1Interface
	clock          clock.Clock
	recorders      map[string]record.EventRecorder
	nodeRef        *v1.ObjectReference
	eventNamespace string
}

// NewClientOrDie creates a new problem client, panics if error occurs.
func NewClientOrDie(npdo *options.NodeProblemDetectorOptions) Client {
	c := &nodeProblemClient{clock: clock.RealClock{}}

	// we have checked it is a valid URI after command line argument is parsed.:)
	uri, _ := url.Parse(npdo.ApiServerOverride)

	cfg, err := getKubeClientConfig(uri)
	if err != nil {
		panic(err)
	}

	cfg.UserAgent = fmt.Sprintf("%s/%s", filepath.Base(os.Args[0]), version.Version())
	// TODO(random-liu): Set QPS Limit
	c.client = clientset.NewForConfigOrDie(cfg).CoreV1()
	c.nodeName = npdo.NodeName
	c.eventNamespace = npdo.EventNamespace
	c.nodeRef = getNodeRef(c.eventNamespace, c.nodeName)
	c.recorders = make(map[string]record.EventRecorder)
	return c
}

func (c *nodeProblemClient) GetConditions(ctx context.Context, conditionTypes []v1.NodeConditionType) ([]*v1.NodeCondition, error) {
	node, err := c.GetNode(ctx)
	if err != nil {
		return nil, err
	}
	conditions := []*v1.NodeCondition{}
	for _, conditionType := range conditionTypes {
		for _, condition := range node.Status.Conditions {
			if condition.Type == conditionType {
				conditions = append(conditions, &condition)
			}
		}
	}
	return conditions, nil
}

func (c *nodeProblemClient) SetConditions(ctx context.Context, newConditions []v1.NodeCondition) error {
	for i := range newConditions {
		// Each time we update the conditions, we update the heart beat time
		newConditions[i].LastHeartbeatTime = metav1.NewTime(c.clock.Now())
	}
	patch, err := generatePatch(newConditions)
	if err != nil {
		return err
	}
	return c.client.RESTClient().Patch(types.StrategicMergePatchType).Resource("nodes").Name(c.nodeName).SubResource("status").Body(patch).Do(ctx).Error()
}

func (c *nodeProblemClient) Eventf(eventType, source, reason, messageFmt string, args ...interface{}) {
	recorder, found := c.recorders[source]
	if !found {
		// TODO(random-liu): If needed use separate client and QPS limit for event.
		recorder = getEventRecorder(c.client, c.eventNamespace, c.nodeName, source)
		c.recorders[source] = recorder
	}
	recorder.Eventf(c.nodeRef, eventType, reason, messageFmt, args...)
}

func (c *nodeProblemClient) GetNode(ctx context.Context) (*v1.Node, error) {
	return c.client.Nodes().Get(ctx, c.nodeName, metav1.GetOptions{})
}

// generatePatch generates condition patch
func generatePatch(conditions []v1.NodeCondition) ([]byte, error) {
	raw, err := json.Marshal(&conditions)
	if err != nil {
		return nil, err
	}
	return []byte(fmt.Sprintf(`{"status":{"conditions":%s}}`, raw)), nil
}

// getEventRecorder generates a recorder for specific node name and source.
func getEventRecorder(c typedcorev1.CoreV1Interface, namespace, nodeName, source string) record.EventRecorder {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(glog.V(4).Infof)
	recorder := eventBroadcaster.NewRecorder(runtime.NewScheme(), v1.EventSource{Component: source, Host: nodeName})
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: c.Events(namespace)})
	return recorder
}

func getNodeRef(namespace, nodeName string) *v1.ObjectReference {
	// TODO(random-liu): Get node to initialize the node reference
	return &v1.ObjectReference{
		Kind:      "Node",
		Name:      nodeName,
		UID:       types.UID(nodeName),
		Namespace: namespace,
	}
}
