/*
Copyright 2026.

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

package controller

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	infrav1alpha1 "github.com/KhangPham05/cloudforge/api/v1alpha1"
	"github.com/KhangPham05/cloudforge/internal/awsclient"
)

// vpcFinalizer marks a VPC as having real AWS state that needs
// cleanup before Kubernetes is allowed to actually delete the object.
const vpcFinalizer = "infra.cloudforge.dev/vpc-finalizer"

// VPCReconciler reconciles a VPC object
type VPCReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	AWSClient awsclient.VPCProvisioner
}

// +kubebuilder:rbac:groups=infra.cloudforge.dev,resources=vpcs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infra.cloudforge.dev,resources=vpcs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infra.cloudforge.dev,resources=vpcs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.25.0/pkg/reconcile
func (r *VPCReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var vpc infrav1alpha1.VPC
	if err := r.Get(ctx, req.NamespacedName, &vpc); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch VPC")
		return ctrl.Result{}, err
	}

	if !vpc.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&vpc, vpcFinalizer) {
			if err := r.AWSClient.DeleteVPC(ctx, vpc.Status.VpcId); err != nil {
				log.Error(err, "deleting VPC", "vpcId", vpc.Status.VpcId)
				return ctrl.Result{}, err
			}
			log.Info("deleted VPC", "vpcId", vpc.Status.VpcId)

			controllerutil.RemoveFinalizer(&vpc, vpcFinalizer)
			if err := r.Update(ctx, &vpc); err != nil {
				log.Error(err, "removing finalizer")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(&vpc, vpcFinalizer) {
		controllerutil.AddFinalizer(&vpc, vpcFinalizer)
		if err := r.Update(ctx, &vpc); err != nil {
			log.Error(err, "adding finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	vpcID, found, err := r.AWSClient.FindVPCByName(ctx, vpc.Spec.VPCName)
	if err != nil {
		log.Error(err, "checking if VPC exists", "vpcName", vpc.Spec.VPCName)
		return ctrl.Result{}, err
	}

	if !found {
		enableDNSSupport := true
		if vpc.Spec.EnableDnsSupport != nil {
			enableDNSSupport = *vpc.Spec.EnableDnsSupport
		}

		vpcID, err = r.AWSClient.CreateVPC(ctx, vpc.Spec.VPCName, vpc.Spec.CidrBlock, enableDNSSupport)
		if err != nil {
			log.Error(err, "creating VPC", "vpcName", vpc.Spec.VPCName)
			return ctrl.Result{}, err
		}
		log.Info("created VPC", "vpcName", vpc.Spec.VPCName, "vpcId", vpcID)
	}

	vpc.Status.VpcId = vpcID
	vpc.Status.Phase = phaseReady
	if err := r.Status().Update(ctx, &vpc); err != nil {
		log.Error(err, "updating VPC status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *VPCReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1alpha1.VPC{}).
		Named("vpc").
		Complete(r)
}
