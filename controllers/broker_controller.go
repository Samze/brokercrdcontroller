/*

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

package controllers

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	osb "github.com/kubernetes-sigs/go-open-service-broker-client/v2"
	broker "github.com/samze/brokercrdcontroller/api/v1alpha1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const domain = "servicebrokers.vmware.com"

// BrokerReconciler reconciles a Broker object
type BrokerReconciler struct {
	Ctrl controller.Controller
	client.Client
	Scheme          *runtime.Scheme
	Log             logr.Logger
	ServicePlanCRDs []ServicePlanCRD
	OSBClient       osb.Client
	StopChan        <-chan struct{}
}

type ServicePlanCRD struct {
	Service osb.Service
	Plan    osb.Plan
	CRD     runtime.Unstructured
}

// +kubebuilder:rbac:groups=broker.servicebrokers.vmware.com,resources=brokers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=broker.servicebrokers.vmware.com,resources=brokers/status,verbs=get;update;patch

func (r *BrokerReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	l := r.Log.WithValues("broker", req.NamespacedName)
	l.Info("Reconciled")

	broker := &broker.Broker{}
	err := r.Get(ctx, req.NamespacedName, broker)
	if err != nil {
		return ctrl.Result{}, err
	}

	return r.ReconcileBroker(req)
}

func (r *BrokerReconciler) ReconcileBroker(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	l := r.Log.WithValues("broker", req.NamespacedName)
	broker := &broker.Broker{}

	if err := r.Get(ctx, req.NamespacedName, broker); err != nil {
		return ctrl.Result{}, err
	}

	catalog, err := r.fetchCatalog(broker.Spec.URL, broker.Spec.Username, broker.Spec.Password)
	if err != nil {
		l.Info("error fetching catalog")
		return ctrl.Result{}, err
	}

	b := BrokerCRDCreator{
		Client: r,
	}

	for _, service := range catalog.Services {
		for _, plan := range service.Plans {
			gvk, err := b.createCRDFromServicePlan(service, plan)
			if err != nil {
				l.Info("error creating crd for service", "service", service.Name, "plan", plan.Name, "err", err)
				return ctrl.Result{}, err
			}

			l.Info("looking out resource", "kind", gvk.Kind)

			unstruct := getUnstructured(gvk)
			crd := ServicePlanCRD{
				Service: service,
				Plan:    plan,
				CRD:     unstruct,
			}

			mgr, err := r.getNewManager()
			if err != nil {
				return ctrl.Result{}, err
			}

			if err := (&DynamicReconciler{
				Client:         mgr.GetClient(),
				Log:            ctrl.Log.WithName("controllers").WithName("Broker"),
				ServicePlanCRD: crd,
				OSBClient:      r.OSBClient,
			}).SetupWithManager(mgr); err != nil {
				l.Info("problem setting up manager", err)
			}

			go func() {
				if err := mgr.Start(r.StopChan); err != nil {
					l.Info("problem running manager", err)
				}
			}()
		}
	}

	return ctrl.Result{}, nil
}

func (r *BrokerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := apiextensionsv1beta1.AddToScheme(r.Scheme); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).For(&broker.Broker{}).Complete(r)
}

type BrokerCRDCreator struct {
	Client client.Client
}

func (b *BrokerCRDCreator) createCRDFromServicePlan(service osb.Service, plan osb.Plan) (metav1.GroupVersionKind, error) {
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

func (r *BrokerReconciler) fetchCatalog(url, username, password string) (*osb.CatalogResponse, error) {
	config := osb.DefaultClientConfiguration()
	config.AuthConfig = &osb.AuthConfig{
		BasicAuthConfig: &osb.BasicAuthConfig{
			Username: username,
			Password: password,
		},
	}
	config.URL = url

	client, err := osb.NewClient(config)
	if err != nil {
		return nil, err
	}

	//eek
	r.OSBClient = client

	cat, err := client.GetCatalog()
	if err != nil {
		return nil, err
	}
	return cat, nil
}

func getUnstructured(gvk metav1.GroupVersionKind) *unstructured.Unstructured {
	unstructured := &unstructured.Unstructured{}

	unstructured.SetGroupVersionKind(schema.GroupVersionKind{Group: gvk.Group, Version: gvk.Version, Kind: gvk.Kind})
	return unstructured
}

func (r *BrokerReconciler) getNewManager() (ctrl.Manager, error) {
	return ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             r.Scheme,
		MetricsBindAddress: "0", //turns off metric server
	})
}
