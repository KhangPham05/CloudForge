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
	"fmt"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	infrav1alpha1 "github.com/KhangPham05/cloudforge/api/v1alpha1"
	"github.com/KhangPham05/cloudforge/internal/awsclient"
)

// rdsInstanceFinalizer marks an RDSInstance as having real AWS state
// that needs cleanup before Kubernetes is allowed to actually delete
// the object.
const rdsInstanceFinalizer = "infra.cloudforge.dev/rdsinstance-finalizer"

// RDSInstanceReconciler reconciles a RDSInstance object
type RDSInstanceReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	AWSClient awsclient.RDSProvisioner
}

// +kubebuilder:rbac:groups=infra.cloudforge.dev,resources=rdsinstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infra.cloudforge.dev,resources=rdsinstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infra.cloudforge.dev,resources=rdsinstances/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create

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
		if controllerutil.ContainsFinalizer(&rds, rdsInstanceFinalizer) {
			// Check-exists first: the object could be deleted before
			// a database was ever actually created (e.g. while still
			// waiting on its VPC), in which case there's nothing to
			// clean up in AWS at all.
			_, found, err := r.AWSClient.DescribeDBInstance(ctx, rds.Name)
			if err != nil {
				log.Error(err, "checking if DB instance exists before delete", "name", rds.Name)
				return ctrl.Result{}, err
			}
			if found {
				if err := r.AWSClient.DeleteDBInstance(ctx, rds.Name); err != nil {
					log.Error(err, "deleting DB instance", "name", rds.Name)
					return ctrl.Result{}, err
				}
				log.Info("deleted DB instance", "name", rds.Name)
			}
			// The credentials Secret has an OwnerReference pointing at
			// this object - Kubernetes garbage-collects it on its own
			// once this object is actually gone, no manual delete needed.

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

	info, found, err := r.AWSClient.DescribeDBInstance(ctx, rds.Name)
	if err != nil {
		log.Error(err, "checking if DB instance exists", "name", rds.Name)
		return ctrl.Result{}, err
	}

	if !found {
		info, err = r.AWSClient.CreateDBInstance(ctx, rds.Name, rds.Spec.Engine, rds.Spec.InstanceClass, rds.Spec.AllocatedStorage)
		if err != nil {
			log.Error(err, "creating DB instance", "name", rds.Name)
			return ctrl.Result{}, err
		}
		log.Info("started creating DB instance", "name", rds.Name)

		// info.Password is only ever populated right here, in this
		// CreateDBInstance response - DescribeDBInstance never returns
		// it (see the interface's own doc comment). Capture it into a
		// Secret immediately, regardless of whether the instance has
		// finished provisioning yet - waiting until it's "available"
		// would mean reading it back via Describe, which never has it.
		if err := r.ensureCredentialsSecret(ctx, &rds, info); err != nil {
			log.Error(err, "creating credentials secret", "name", rds.Name)
			return ctrl.Result{}, err
		}
	}

	if info.State != awsclient.RDSInstanceStateAvailable {
		log.Info("DB instance still provisioning, requeuing", "name", rds.Name, "state", info.State)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	rds.Status.Endpoint = info.Endpoint
	rds.Status.Port = info.Port
	rds.Status.Phase = phaseReady
	rds.Status.SecretRef = credentialsSecretName(rds.Name)
	if err := r.Status().Update(ctx, &rds); err != nil {
		log.Error(err, "updating RDSInstance status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// credentialsSecretName is deterministic from the RDSInstance's own
// name, so it never needs to be remembered separately - any code that
// knows the RDSInstance's name can compute its Secret's name too.
func credentialsSecretName(rdsName string) string {
	return rdsName + "-credentials"
}

// ensureCredentialsSecret creates the Kubernetes Secret holding this
// database's connection details, if it doesn't already exist. Owned
// by the RDSInstance, so Kubernetes garbage-collects it automatically
// when the RDSInstance is deleted.
func (r *RDSInstanceReconciler) ensureCredentialsSecret(ctx context.Context, rds *infrav1alpha1.RDSInstance, info awsclient.RDSInstanceInfo) error {
	name := credentialsSecretName(rds.Name)

	existing := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{Namespace: rds.Namespace, Name: name}, existing)
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rds.Namespace,
		},
		StringData: map[string]string{
			"DB_HOST":     info.Endpoint,
			"DB_PORT":     strconv.Itoa(int(info.Port)),
			"DB_PASSWORD": info.Password,
		},
	}

	if err := controllerutil.SetControllerReference(rds, secret, r.Scheme); err != nil {
		return fmt.Errorf("setting owner reference on credentials secret: %w", err)
	}

	if err := r.Create(ctx, secret); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RDSInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1alpha1.RDSInstance{}).
		Named("rdsinstance").
		Complete(r)
}
