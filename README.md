# Brokercrdcontroller

The current way to consume OSBAPI services on k8s is [Service Catalog](https://github.com/kubernetes-sigs/service-catalog). 

Service Catalog provides a general purpose `ServiceInstance` CRD that you represents the instantiation of any type of Service. This is in contrast to there being a CRD for each service, which is how Operators work.

This PoC translates Services found in OSBAPI Catalogs to K8s CRDS dynamically (on broker registration).

Go from an OSBAPI Service Broker Catalog that looks like this.
```
{
  "services": [
    {
      "name": "rabbitmq",
      "description": "Provides an overview of any service instances and bindings that have been created by a platform.",
      "id": "7cd85cd7-700d-4ca1-98e7-ffe82751dfae",
      "plans": [
        {
          "name": "ha",
          "description": "A small instance of the service.",
          "id": "bc27fed8-e606-4064-856b-94fedc966078"
        },
        {
          "name": "perf",
          "description": "A large instance of the service.",
          "id": "6467cf08-ee3d-4083-af6e-8bf3d1b03de9"
           "schemas": {
            "service_instance": {
              "create": {
                "parameters": {
                  "$schema": "http://json-schema.org/draft-04/schema#",
                  "additionalProperties": false,
                  "type": "object",
                   "color": {
                      "type": "string",
                      "enum": [
                        "red",
                        "amber",
                      ],
                      "default": "red",
                      "description": "Your favourite color"
                    },
                 }
              }
           }
        }
      ]
    }
  ]
}
```

That on OSBAPI Broker registration dynamically creates CRDs that a user can then create an instance of in k8s like this:

```
apiVersion: servicebrokers.vmware.com/v1alpha1
kind: RabbitMQHa
metadata:
  name: mysmallservice
```

```
apiVersion: servicebrokers.vmware.com/v1alpha1
kind: RabbitMQPerf
metadata:
  name: mylargeservice
spec:
  color: red
```

### Usage

1. make install && make run

1. Register the broker with the broker CR:
```
apiVersion: broker.servicebrokers.vmware.com/v1alpha1
kind: Broker
metadata:
  name: broker-sample
spec:
  URL: http://localhost:8080
  username: USERNAME
  password: PASSWORD
```

2. New CRDs will appear in the cluster

### Notes

* Creates a new CRD for each service + plan combination.
* Maps OSBAPI create schema parameters to CRD Structural scheme.
* Does not model binding
