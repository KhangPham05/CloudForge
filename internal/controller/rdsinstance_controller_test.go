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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	infrav1alpha1 "github.com/KhangPham05/cloudforge/api/v1alpha1"
)

var _ = Describe("RDSInstance Controller", func() {
	Context("When reconciling a resource", func() {
		const (
			resourceName      = "test-resource"
			resourceNamespace = "default"
			testVpcRefName    = "some-vpc"
		)

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: resourceNamespace,
		}
		rdsinstance := &infrav1alpha1.RDSInstance{}

		var controllerReconciler *RDSInstanceReconciler

		// reconcileTwice drives an RDSInstance through the same
		// two-step finalizer-attach dance as S3Bucket/VPC: the first
		// call only adds the finalizer, the second is what actually
		// reaches the VPC cross-resource lookup logic.
		reconcileTwice := func() (reconcile.Result, error) {
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())
			return controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		}

		BeforeEach(func() {
			controllerReconciler = &RDSInstanceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			By("creating the custom resource for the Kind RDSInstance")
			err := k8sClient.Get(ctx, typeNamespacedName, rdsinstance)
			if err != nil && apierrors.IsNotFound(err) {
				resource := &infrav1alpha1.RDSInstance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: resourceNamespace,
					},
					Spec: infrav1alpha1.RDSInstanceSpec{
						InstanceClass:    "db.t3.micro",
						Engine:           "postgres",
						AllocatedStorage: 20,
						VpcRef:           testVpcRefName,
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &infrav1alpha1.RDSInstance{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if apierrors.IsNotFound(err) {
				return
			}
			Expect(err).NotTo(HaveOccurred())

			By("deleting the RDSInstance resource instance")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should requeue when the referenced VPC does not exist", func() {
			result, err := reconcileTwice()
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))
		})

		It("should requeue when the referenced VPC exists but is not Ready", func() {
			vpc := &infrav1alpha1.VPC{
				ObjectMeta: metav1.ObjectMeta{Name: testVpcRefName, Namespace: resourceNamespace},
				Spec: infrav1alpha1.VPCSpec{
					VPCName:   "test-vpc",
					CidrBlock: "10.0.0.0/16",
				},
			}
			Expect(k8sClient.Create(ctx, vpc)).To(Succeed())
			defer func() {
				Expect(k8sClient.Delete(ctx, vpc)).To(Succeed())
			}()

			result, err := reconcileTwice()
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))
		})

		It("should stop requeuing once the referenced VPC is Ready", func() {
			vpc := &infrav1alpha1.VPC{
				ObjectMeta: metav1.ObjectMeta{Name: testVpcRefName, Namespace: resourceNamespace},
				Spec: infrav1alpha1.VPCSpec{
					VPCName:   "test-vpc",
					CidrBlock: "10.0.0.0/16",
				},
			}
			Expect(k8sClient.Create(ctx, vpc)).To(Succeed())
			defer func() {
				Expect(k8sClient.Delete(ctx, vpc)).To(Succeed())
			}()

			vpc.Status.Phase = phaseReady
			vpc.Status.VpcId = "vpc-test12345"
			Expect(k8sClient.Status().Update(ctx, vpc)).To(Succeed())

			result, err := reconcileTwice()
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())
		})
	})
})
