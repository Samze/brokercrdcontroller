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
	"github.com/google/uuid"
	broker "github.com/samze/brokercrdcontroller/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	osb "github.com/kubernetes-sigs/go-open-service-broker-client/v2"
	ctrl "sigs.k8s.io/controller-runtime"
)

// DynamicReconciler reconciles a Broker object
type DynamicReconciler struct {
	client.Client
	Log            logr.Logger
	ServicePlanCRD ServicePlanCRD
	OSBClient      osb.Client
}

type ServicePlanCRD struct {
	Service      osb.Service
	Plan         osb.Plan
	Unstructured runtime.Unstructured
	Broker       *broker.Broker
}

// +kubebuilder:rbac:groups=broker.servicebrokers.vmware.com,resources=brokers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=broker.servicebrokers.vmware.com,resources=brokers/status,verbs=get;update;patch

func (r *DynamicReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	name := r.ServicePlanCRD.Service.Name + r.ServicePlanCRD.Plan.Name
	l := r.Log.WithValues("dynamic for:", name, "req", req.NamespacedName)
	l.Info("Reconciled")

	resource := r.ServicePlanCRD.Unstructured
	if err := r.Get(ctx, req.NamespacedName, resource); err != nil {
		return ctrl.Result{}, err
	}

	if isProvisioned(resource) {
		l.Info("Already provisioned")
		return ctrl.Result{}, nil
	}

	uuid, _ := uuid.NewUUID()
	_, err := r.OSBClient.ProvisionInstance(&osb.ProvisionRequest{
		InstanceID:       uuid.String(),
		ServiceID:        r.ServicePlanCRD.Service.ID,
		PlanID:           r.ServicePlanCRD.Plan.ID,
		OrganizationGUID: "something",
		SpaceGUID:        "something",
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	setProvisioned(resource)

	l.Info("provisioned")
	if err := r.Update(ctx, resource); err != nil {
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

func (r *DynamicReconciler) SetupWithManager(mgr ctrl.Manager) error {
	unstructured := r.ServicePlanCRD.Unstructured
	return ctrl.NewControllerManagedBy(mgr).For(unstructured).Complete(r)
}

func setProvisioned(resource runtime.Unstructured) {
	content := resource.UnstructuredContent()
	provisioned := map[string]interface{}{
		"provisioned": true,
	}
	content["status"] = provisioned
}

func isProvisioned(resource runtime.Unstructured) bool {
	content := resource.UnstructuredContent()
	status, ok := content["status"].(map[string]interface{})
	if !ok {
		return false
	}
	v, ok := status["provisioned"]
	if !ok || v == false {
		return false
	}
	return true

}
