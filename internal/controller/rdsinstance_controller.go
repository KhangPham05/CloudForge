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
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	infrav1alpha1 "github.com/KhangPham05/cloudforge/api/v1alpha1"
)

// rdsInstanceFinalizer marks an RDSInstance as having real AWS state
// that needs cleanup before Kubernetes is allowed to actually delete
// the object.
const rdsInstanceFinalizer = "infra.cloudforge.dev/rdsinstance-finalizer"

// RDSInstanceReconciler reconciles a RDSInstance object
type RDSInstanceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=infra.cloudforge.dev,resources=rdsinstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infra.cloudforge.dev,resources=rdsinstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infra.cloudforge.dev,resources=rdsinstances/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the RDSInstance object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.25.0/pkg/reconcile
func (r *RDSInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var rds infrav1alpha1.RDSInstance
	if err := r.Get(ctx, req.NamespacedName, &rds); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch RDSInstance")
		return ctrl.Result{}, err
	}

	if !rds.DeletionTimestamp.IsZero() {
		// Nothing has been provisioned yet in Phase 11 - once Phase 12
		// adds real provisioning, this branch will call DeleteDBInstance
		// before removing the finalizer, same as S3Bucket/VPC.
		if controllerutil.ContainsFinalizer(&rds, rdsInstanceFinalizer) {
			controllerutil.RemoveFinalizer(&rds, rdsInstanceFinalizer)
			if err := r.Update(ctx, &rds); err != nil {
				log.Error(err, "removing finalizer")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(&rds, rdsInstanceFinalizer) {
		controllerutil.AddFinalizer(&rds, rdsInstanceFinalizer)
		if err := r.Update(ctx, &rds); err != nil {
			log.Error(err, "adding finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	var vpc infrav1alpha1.VPC
	vpcKey := client.ObjectKey{Namespace: rds.Namespace, Name: rds.Spec.VpcRef}
	if err := r.Get(ctx, vpcKey, &vpc); err != nil {
		if apierrors.IsNotFound(err) {
			// The referenced VPC doesn't exist (yet, or ever, if it
			// was a typo) - not a fetch error on our own object, so
			// don't treat it like one. Check back later rather than
			// hot-looping on something that might never resolve.
			log.Info("referenced VPC not found, requeuing", "vpcRef", rds.Spec.VpcRef)
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		log.Error(err, "unable to fetch referenced VPC", "vpcRef", rds.Spec.VpcRef)
		return ctrl.Result{}, err
	}

	if vpc.Status.Phase != phaseReady {
		log.Info("referenced VPC not Ready yet, requeuing", "vpcRef", rds.Spec.VpcRef, "vpcPhase", vpc.Status.Phase)
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	// TODO(Phase 12): provision the database, generate the credentials Secret.

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RDSInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1alpha1.RDSInstance{}).
		Named("rdsinstance").
		Complete(r)
}
