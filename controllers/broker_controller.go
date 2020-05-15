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

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/samze/brokercrdcontroller/pkg/osbapi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	osb "github.com/kubernetes-sigs/go-open-service-broker-client/v2"
	broker "github.com/samze/brokercrdcontroller/api/v1alpha1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

// BrokerReconciler reconciles a Broker object
type BrokerReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Log       logr.Logger
	OSBClient osb.Client
	StopChan  <-chan struct{}
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
		if errors.IsNotFound(err) {
			//deleted
			return ctrl.Result{}, nil
		}
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

	osbclient, err := osbapi.GetClient(broker.Spec.URL, broker.Spec.Username, broker.Spec.Password)
	if err != nil {
		l.Info("error creating osbapi client")
		return ctrl.Result{}, err
	}

	catalog, err := osbclient.GetCatalog()
	if err != nil {
		l.Info("error fetching catalog")
		return ctrl.Result{}, err
	}

	crdCreator := osbapi.BrokerCRDCreator{
		Client: r,
	}

	for _, service := range catalog.Services {
		for _, plan := range service.Plans {
			gvk, err := crdCreator.Create(service, plan)
			if err != nil {
				l.Info("error creating crd for service", "service", service.Name, "plan", plan.Name, "err", err)
				return ctrl.Result{}, err
			}

			unstruct := getUnstructured(gvk)
			crd := ServicePlanCRD{
				Service:      service,
				Plan:         plan,
				Unstructured: unstruct,
				Broker:       broker,
			}

			if err := r.setupManagerForCRD(osbclient, crd); err != nil {
				return ctrl.Result{}, nil
			}
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

func (r *BrokerReconciler) setupManagerForCRD(osbclient osb.Client, crd ServicePlanCRD) error {
	mgr, err := r.getNewManager()
	if err != nil {
		return err
	}

	if err := (&DynamicReconciler{
		Client:         mgr.GetClient(),
		Log:            ctrl.Log.WithName("controllers").WithName("Broker"),
		ServicePlanCRD: crd,
		OSBClient:      osbclient,
	}).SetupWithManager(mgr); err != nil {
		r.Log.Info("problem setting up manager", err)
	}

	go func() {
		if err := mgr.Start(r.StopChan); err != nil {
			r.Log.Info("problem running manager", err)
		}
	}()
	return nil
}
