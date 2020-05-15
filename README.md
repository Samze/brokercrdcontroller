# Brokercrdcontroller

Go from OSBAPI Catalogs to Native K8s CRDS.


Go from a Service Broker Catalog that looks like this.
```
{
  "services": [
    {
      "name": "overview-service",
      "description": "Provides an overview of any service instances and bindings that have been created by a platform.",
      "id": "7cd85cd7-700d-4ca1-98e7-ffe82751dfae",
      "plans": [
        {
          "name": "small",
          "description": "A small instance of the service.",
          "id": "bc27fed8-e606-4064-856b-94fedc966078"
        },
        {
          "name": "large",
          "description": "A large instance of the service.",
          "id": "6467cf08-ee3d-4083-af6e-8bf3d1b03de9"
          "schemas": {
            "service_instance": {
              "create": {
                "parameters": {
                  "$schema": "http://json-schema.org/draft-04/schema#",
                  "additionalProperties": false,
                  "type": "object",
                  "properties": {
                    "color": {
                      "type": "string",
                      "enum": [
                        "red",
                        "amber",
                        "green"
                      ],
                      "default": "green",
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

To CRDs that can be created in k8s like this

```
apiVersion: servicebrokers.vmware.com/v1alpha1
kind: OverviewServiceSmall
metadata:
  name: mysmallservice
```

```
apiVersion: servicebrokers.vmware.com/v1alpha1
kind: OverviewServiceLarge
metadata:
  name: mylargeservice
spec:
  color: red
```

>>>>>>> lil readme
