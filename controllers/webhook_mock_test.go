package controllers

import "net/http"

func setupMockHandlersForWebhook() {
	server.RouteToHandler("POST", "/terraform/admission/pass", func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Content-Type") != "application/json" {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}

		writer.WriteHeader(http.StatusOK)
		_, err := writer.Write([]byte(`{ "passed": true, "violations": [] }`))
		if err != nil {
			return
		}
	})

	// define mock handlers for webhook server
	server.RouteToHandler("POST", "/terraform/admission/fail", func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Content-Type") != "application/json" {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}

		writer.WriteHeader(http.StatusOK)
		_, err := writer.Write([]byte(`
{
	"passed": false,
	"violations": [
		{
			"id": "fa4a2c4c-fb3d-4bd3-b11a-e00085939521",
			"account_id": "",
			"cluster_id": "",
			"policy": {
				"name": "Max nodes count in GCP",
				"id": "weave.policies.max-node-count",
				"code": "...",
				"enabled": true,
				"parameters": [
					{
						"name": "max_node_size",
						"type": "integer",
						"value": 0,
						"required": true
					}
				],
				"targets": {
					"kinds": [
						"Terraform"
					],
					"labels": null,
					"namespaces": null
				},
				"description": "Max nodes count in GCP",
				"how_to_solve": "decrease number of nodes",
				"category": "weave.categories.access-control",
				"tags": null,
				"severity": "high",
				"standards": null,
				"provider": ""
			},
			"entity": {
				"id": "",
				"name": "helloworld",
				"apiVersion": "infra.contrib.fluxcd.io/v1alpha2",
				"kind": "Terraform",
				"namespace": "default",
				"manifest": {
					"apiVersion": "infra.contrib.fluxcd.io/v1alpha2",
					"kind": "Terraform",
					"metadata": {
						"name": "helloworld",
						"namespace": "default"
					},
					"spec": {
						"path": "./tf"
					},
					"status": null
				},
				"resource_version": "",
				"has_parent": false
			},
			"status": "Violation",
			"message": "Max nodes count in GCP in terraform helloworld (1 occurrences)",
			"occurrences": [
				{
					"message": "You are trying to provision 1 node(s). The policy is set for a maximum of 0 nodes."
				}
			],
			"source": "Admission",
			"trigger": "terraform",
			"created_at": "2022-08-30T13:17:33.342986364Z",
			"metadata": null
		}
	]
}
`))
		if err != nil {
			return
		}
	})
}
