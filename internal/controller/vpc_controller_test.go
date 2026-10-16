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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	infrav1alpha1 "github.com/KhangPham05/cloudforge/api/v1alpha1"
	"github.com/KhangPham05/cloudforge/internal/awsclient/fake"
)

var _ = Describe("VPC Controller", func() {
	Context("When reconciling a resource", func() {
		const (
			resourceName      = "test-resource"
			resourceNamespace = "default"
			testVPCName       = "cloudforge-envtest-vpc"
			testCidrBlock     = "10.0.0.0/16"
		)

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: resourceNamespace,
		}
		vpc := &infrav1alpha1.VPC{}

		var (
			awsClient            *fake.VPCProvisioner
			controllerReconciler *VPCReconciler
		)

		BeforeEach(func() {
			awsClient = fake.NewVPCProvisioner()
			controllerReconciler = &VPCReconciler{
				Client:    k8sClient,
				Scheme:    k8sClient.Scheme(),
				AWSClient: awsClient,
			}

			By("creating the custom resource for the Kind VPC")
			err := k8sClient.Get(ctx, typeNamespacedName, vpc)
			if err != nil && apierrors.IsNotFound(err) {
				resource := &infrav1alpha1.VPC{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: resourceNamespace,
					},
					Spec: infrav1alpha1.VPCSpec{
						VPCName:   testVPCName,
						CidrBlock: testCidrBlock,
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &infrav1alpha1.VPC{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if apierrors.IsNotFound(err) {
				return
			}
			Expect(err).NotTo(HaveOccurred())

			By("deleting the VPC resource instance")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should add a finalizer, then create the VPC and mark it Ready", func() {
			By("reconciling once to add the finalizer")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, typeNamespacedName, vpc)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(vpc, vpcFinalizer)).To(BeTrue())

			By("reconciling again to actually create the VPC")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			vpcID, found, err := awsClient.FindVPCByName(ctx, testVPCName)
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeTrue())
			Expect(vpcID).NotTo(BeEmpty())

			Expect(k8sClient.Get(ctx, typeNamespacedName, vpc)).To(Succeed())
			Expect(vpc.Status.Phase).To(Equal("Ready"))
			Expect(vpc.Status.VpcId).To(Equal(vpcID))
		})

		It("should delete the VPC when the resource is deleted", func() {
			By("reconciling twice to add the finalizer and create the VPC")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("deleting the Kubernetes object")
			Expect(k8sClient.Get(ctx, typeNamespacedName, vpc)).To(Succeed())
			Expect(k8sClient.Delete(ctx, vpc)).To(Succeed())

			By("reconciling once more to run the finalizer cleanup")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("confirming the VPC is gone from the fake AWS client")
			_, found, err := awsClient.FindVPCByName(ctx, testVPCName)
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeFalse())

			By("confirming the Kubernetes object is fully gone")
			err = k8sClient.Get(ctx, typeNamespacedName, vpc)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})
	})
})
