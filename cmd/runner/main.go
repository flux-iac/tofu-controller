/*
Copyright 2021.

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
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	flag "github.com/spf13/pflag"

	"github.com/weaveworks/tf-controller/mtls"
)

/* Please prepare the following envs for this program
   env:
     - name: POD_NAME
       valueFrom:
         fieldRef:
           fieldPath: metadata.name
     - name: POD_NAMESPACE
       valueFrom:
         fieldRef:
           fieldPath: metadata.namespace
*/

func main() {
	var (
		grpcPort int
	)

	flag.IntVar(&grpcPort, "grpc-port", 30000, "The port on which to expose the grpc endpoint.")
	flag.Parse()

	addr := fmt.Sprintf(":%d", grpcPort)

	_ = os.Getenv("POD_NAME")
	podNamespace := os.Getenv("POD_NAMESPACE")

	// catch the SIGTERM from the kubelet to gracefully terminate
	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGTERM)
	defer func() {
		signal.Stop(sigterm)
	}()

	err := mtls.RunnerServe(podNamespace, addr, sigterm)
	if err != nil {
		log.Fatal(err.Error())
	}
}
