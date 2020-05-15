package osbapi

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"

	osb "github.com/kubernetes-sigs/go-open-service-broker-client/v2"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const domain = "servicebrokers.vmware.com"

type BrokerCRDCreator struct {
	Client client.Client
}

func (b *BrokerCRDCreator) Create(service osb.Service, plan osb.Plan) (metav1.GroupVersionKind, error) {
	namesingular := fixName(strings.Title(service.Name) + strings.Title(plan.Name))
	nameplural := fixName(namesingular + "s")
	name := strings.ToLower(nameplural) + "." + domain

	crd := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
			Group: domain,
			Scope: "Cluster",
			Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
				Kind:     namesingular,
				ListKind: namesingular + "List",
				Plural:   strings.ToLower(nameplural),
				Singular: strings.ToLower(namesingular),
			},
			Version: "v1alpha1",
		},
	}

	if plan.Schemas != nil && plan.Schemas.ServiceInstance != nil && plan.Schemas.ServiceInstance.Create != nil && plan.Schemas.ServiceInstance.Create.Parameters != nil {
		planSchema := formatJSONSchemaProps(plan.Schemas.ServiceInstance.Create.Parameters)

		crd.Spec.Validation = &apiextensionsv1beta1.CustomResourceValidation{
			OpenAPIV3Schema: planSchema,
		}
	}

	if err := b.Client.Get(context.TODO(), types.NamespacedName{Name: name}, crd); err != nil {
		if errors.IsNotFound(err) {
			if err := b.Client.Create(context.TODO(), crd); err != nil {
				return metav1.GroupVersionKind{}, err
			}
		} else {
			return metav1.GroupVersionKind{}, err
		}
	}

	return metav1.GroupVersionKind{
		Group:   domain,
		Version: "v1alpha1",
		Kind:    namesingular,
	}, nil
}

func formatJSONSchemaProps(schema interface{}) *apiextensionsv1beta1.JSONSchemaProps {
	b, err := json.Marshal(schema)
	if err != nil {
		log.Fatalf("marshal boom %v", err)
	}

	props := &apiextensionsv1beta1.JSONSchemaProps{}

	err = json.Unmarshal(b, props)
	if err != nil {
		log.Fatalf("marshal boom %v", err)
	}

	props.AdditionalProperties = nil
	props.Schema = ""

	outerProps := &apiextensionsv1beta1.JSONSchemaProps{
		Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
			"spec": *props,
		},
	}
	return outerProps
}

func fixName(n string) string {
	return strings.ReplaceAll(n, "-", "")
}
