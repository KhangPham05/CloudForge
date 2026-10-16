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

var _ = Describe("S3Bucket Controller", func() {
	Context("When reconciling a resource", func() {
		const (
			resourceName      = "test-resource"
			resourceNamespace = "default"
			testBucketName    = "cloudforge-envtest-bucket"
		)

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: resourceNamespace,
		}
		s3bucket := &infrav1alpha1.S3Bucket{}

		// awsClient and controllerReconciler are rebuilt fresh in
		// BeforeEach for every test, so each test's fake AWS state
		// starts empty and tests never leak state into each other.
		var (
			awsClient            *fake.S3Provisioner
			controllerReconciler *S3BucketReconciler
		)

		BeforeEach(func() {
			awsClient = fake.NewS3Provisioner()
			controllerReconciler = &S3BucketReconciler{
				Client:    k8sClient,
				Scheme:    k8sClient.Scheme(),
				AWSClient: awsClient,
			}

			By("creating the custom resource for the Kind S3Bucket")
			err := k8sClient.Get(ctx, typeNamespacedName, s3bucket)
			if err != nil && apierrors.IsNotFound(err) {
				resource := &infrav1alpha1.S3Bucket{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: resourceNamespace,
					},
					Spec: infrav1alpha1.S3BucketSpec{
						BucketName: testBucketName,
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &infrav1alpha1.S3Bucket{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if apierrors.IsNotFound(err) {
				// Already fully deleted by the test itself - nothing
				// left to clean up.
				return
			}
			Expect(err).NotTo(HaveOccurred())

			By("deleting the S3Bucket resource instance")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())

			// Drive the finalizer's cleanup to completion using the
			// same fake AWS client this test used, in case this
			// particular test never reconciled all the way through
			// deletion itself.
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should add a finalizer, then create the bucket and mark it Ready", func() {
			By("reconciling once to add the finalizer")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, typeNamespacedName, s3bucket)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(s3bucket, s3BucketFinalizer)).To(BeTrue())

			By("reconciling again to actually create the bucket")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			exists, err := awsClient.BucketExists(ctx, testBucketName)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeTrue())

			Expect(k8sClient.Get(ctx, typeNamespacedName, s3bucket)).To(Succeed())
			Expect(s3bucket.Status.Phase).To(Equal("Ready"))
		})

		It("should empty and delete the bucket when the resource is deleted", func() {
			By("reconciling twice to add the finalizer and create the bucket")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("putting a real object in the fake bucket, so emptying has real work to do")
			awsClient.PutObject(testBucketName, "hello.txt")

			By("deleting the Kubernetes object")
			Expect(k8sClient.Get(ctx, typeNamespacedName, s3bucket)).To(Succeed())
			Expect(k8sClient.Delete(ctx, s3bucket)).To(Succeed())

			By("reconciling once more to run the finalizer cleanup")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("confirming the bucket is gone from the fake AWS client")
			exists, err := awsClient.BucketExists(ctx, testBucketName)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeFalse())

			By("confirming the Kubernetes object is fully gone")
			err = k8sClient.Get(ctx, typeNamespacedName, s3bucket)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})
	})
})
