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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	infrav1alpha1 "github.com/KhangPham05/cloudforge/api/v1alpha1"
	"github.com/KhangPham05/cloudforge/internal/awsclient"
	"github.com/KhangPham05/cloudforge/internal/awsclient/fake"
)

// provisioningDelay is deliberately non-zero, so tests can observe the
// DB instance genuinely sitting in "creating" state for a moment -
// zero would make every create instantly "available" and silently
// skip over the exact window the password-capture bug lived in.
const provisioningDelay = 100 * time.Millisecond

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

		var (
			awsClient            *fake.RDSProvisioner
			controllerReconciler *RDSInstanceReconciler
		)

		// reconcileTwice drives an RDSInstance through the same
		// two-step finalizer-attach dance as S3Bucket/VPC: the first
		// call only adds the finalizer, the second is what actually
		// reaches the VPC cross-resource lookup logic.
		reconcileTwice := func() (reconcile.Result, error) {
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())
			return controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		}

		// createReadyVPC creates a VPC and immediately marks it Ready,
		// so RDS tests that need a satisfied dependency don't each
		// have to repeat the same setup.
		createReadyVPC := func() *infrav1alpha1.VPC {
			vpc := &infrav1alpha1.VPC{
				ObjectMeta: metav1.ObjectMeta{Name: testVpcRefName, Namespace: resourceNamespace},
				Spec: infrav1alpha1.VPCSpec{
					VPCName:   "test-vpc",
					CidrBlock: "10.0.0.0/16",
				},
			}
			Expect(k8sClient.Create(ctx, vpc)).To(Succeed())
			vpc.Status.Phase = phaseReady
			vpc.Status.VpcId = "vpc-test12345"
			Expect(k8sClient.Status().Update(ctx, vpc)).To(Succeed())
			return vpc
		}

		BeforeEach(func() {
			awsClient = fake.NewRDSProvisioner(provisioningDelay)
			controllerReconciler = &RDSInstanceReconciler{
				Client:    k8sClient,
				Scheme:    k8sClient.Scheme(),
				AWSClient: awsClient,
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
			vpc := &infrav1alpha1.VPC{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: testVpcRefName, Namespace: resourceNamespace}, vpc); err == nil {
				Expect(k8sClient.Delete(ctx, vpc)).To(Succeed())
			}

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

			result, err := reconcileTwice()
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))
		})

		It("should stop requeuing once the referenced VPC is Ready", func() {
			createReadyVPC()

			result, err := reconcileTwice()
			Expect(err).NotTo(HaveOccurred())
			// Past the VPC gate now, into provisioning - still not zero,
			// since the DB instance itself takes a moment to become
			// available (see provisioningDelay).
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))
		})

		It("should create a credentials Secret immediately, even while the DB is still provisioning", func() {
			createReadyVPC()

			_, err := reconcileTwice()
			Expect(err).NotTo(HaveOccurred())

			By("confirming the DB instance exists but is not Available yet")
			info, found, err := awsClient.DescribeDBInstance(ctx, resourceName)
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeTrue())
			Expect(info.State).To(Equal(awsclient.RDSInstanceStateCreating))

			By("confirming the RDSInstance itself is NOT Ready yet")
			Expect(k8sClient.Get(ctx, typeNamespacedName, rdsinstance)).To(Succeed())
			Expect(rdsinstance.Status.Phase).NotTo(Equal(phaseReady))

			By("confirming the credentials Secret was created anyway, with real data")
			secret := &corev1.Secret{}
			secretKey := types.NamespacedName{Name: credentialsSecretName(resourceName), Namespace: resourceNamespace}
			Expect(k8sClient.Get(ctx, secretKey, secret)).To(Succeed())
			// StringData is write-only - it never comes back on a Get,
			// only .Data (already base64-decoded into []byte) does.
			Expect(secret.Data["DB_PASSWORD"]).NotTo(BeEmpty())
			Expect(secret.Data["DB_HOST"]).NotTo(BeEmpty())
			Expect(string(secret.Data["DB_PORT"])).To(Equal("5432"))
		})

		It("should set an owner reference on the generated Secret", func() {
			createReadyVPC()
			Expect(k8sClient.Get(ctx, typeNamespacedName, rdsinstance)).NotTo(HaveOccurred())

			_, err := reconcileTwice()
			Expect(err).NotTo(HaveOccurred())

			secret := &corev1.Secret{}
			secretKey := types.NamespacedName{Name: credentialsSecretName(resourceName), Namespace: resourceNamespace}
			Expect(k8sClient.Get(ctx, secretKey, secret)).To(Succeed())

			Expect(secret.OwnerReferences).To(HaveLen(1))
			Expect(secret.OwnerReferences[0].Name).To(Equal(resourceName))
			Expect(secret.OwnerReferences[0].Kind).To(Equal("RDSInstance"))
			Expect(*secret.OwnerReferences[0].Controller).To(BeTrue())
		})

		It("should become Ready once the DB instance finishes provisioning, without recreating the Secret", func() {
			createReadyVPC()

			_, err := reconcileTwice()
			Expect(err).NotTo(HaveOccurred())

			secretKey := types.NamespacedName{Name: credentialsSecretName(resourceName), Namespace: resourceNamespace}
			firstSecret := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, secretKey, firstSecret)).To(Succeed())
			firstPassword := string(firstSecret.Data["DB_PASSWORD"])
			Expect(firstPassword).NotTo(BeEmpty())

			time.Sleep(provisioningDelay * 2)

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())

			By("confirming RDSInstance status is fully populated")
			Expect(k8sClient.Get(ctx, typeNamespacedName, rdsinstance)).To(Succeed())
			Expect(rdsinstance.Status.Phase).To(Equal(phaseReady))
			Expect(rdsinstance.Status.Endpoint).NotTo(BeEmpty())
			Expect(rdsinstance.Status.Port).To(Equal(int32(5432)))
			Expect(rdsinstance.Status.SecretRef).To(Equal(credentialsSecretName(resourceName)))

			By("confirming the Secret was not recreated or changed")
			secondSecret := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, secretKey, secondSecret)).To(Succeed())
			Expect(string(secondSecret.Data["DB_PASSWORD"])).To(Equal(firstPassword))
		})

		It("should delete the DB instance when the resource is deleted", func() {
			createReadyVPC()

			_, err := reconcileTwice()
			Expect(err).NotTo(HaveOccurred())
			time.Sleep(provisioningDelay * 2)
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			_, found, err := awsClient.DescribeDBInstance(ctx, resourceName)
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeTrue())

			Expect(k8sClient.Get(ctx, typeNamespacedName, rdsinstance)).To(Succeed())
			Expect(k8sClient.Delete(ctx, rdsinstance)).To(Succeed())

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("confirming the DB instance is gone from the AWS client")
			_, found, err = awsClient.DescribeDBInstance(ctx, resourceName)
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeFalse())

			By("confirming the RDSInstance object is fully gone")
			err = k8sClient.Get(ctx, typeNamespacedName, rdsinstance)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())

			// Not asserted here: that the Secret itself disappears.
			// envtest runs only kube-apiserver + etcd, not the real
			// garbage-collector controller that watches OwnerReferences
			// and deletes owned objects - that cascade-delete only
			// happens for real in the live kind cluster (verified in
			// the live demo, not here).
		})
	})
})
