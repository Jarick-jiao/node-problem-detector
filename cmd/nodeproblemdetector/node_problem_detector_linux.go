/*
Copyright 2021 The Kubernetes Authors All rights reserved.

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

package main

import (
	"context"

	"github.com/golang/glog"
	"github.com/spf13/pflag"
	"k8s.io/node-problem-detector/cmd/options"
)

func main() {
	npdo := options.NewNodeProblemDetectorOptions()
	npdo.AddFlags(pflag.CommandLine)

	pflag.Parse()
	if err := npdMain(context.Background(), npdo); err != nil {
		glog.Fatalf("Problem detector failed with error: %v", err)
	}
}
