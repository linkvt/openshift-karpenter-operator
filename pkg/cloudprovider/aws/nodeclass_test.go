package aws

import (
	"encoding/json"
	"testing"

	. "github.com/onsi/gomega"

	openshiftkarpenterv1 "github.com/openshift/karpenter-operator/api/karpenter/v1"
)

func TestDefaultNodeClass(t *testing.T) {
	g := NewWithT(t)
	provider := hcpEC2NodeClassProvider{}

	object, mutate, err := provider.DefaultNodeClass()
	g.Expect(err).NotTo(HaveOccurred())

	nodeClass, ok := object.(*openshiftkarpenterv1.OpenshiftEC2NodeClass)
	g.Expect(ok).To(BeTrue())
	nodeClass.Spec.Tags = map[string]string{"user": "override"}
	g.Expect(mutate()).To(Succeed())
	g.Expect(nodeClass.Name).To(Equal("default"))
	g.Expect(nodeClass.APIVersion).To(Equal(openshiftkarpenterv1.SchemeGroupVersion.String()))
	g.Expect(nodeClass.Kind).To(Equal("OpenshiftEC2NodeClass"))
	g.Expect(nodeClass.Labels).To(Equal(map[string]string{
		"app.kubernetes.io/managed-by": "karpenter-operator",
	}))
	g.Expect(nodeClass.Spec).To(Equal(openshiftkarpenterv1.OpenshiftEC2NodeClassSpec{}))

	serialized, err := json.Marshal(nodeClass)
	g.Expect(err).NotTo(HaveOccurred())
	var fields map[string]json.RawMessage
	g.Expect(json.Unmarshal(serialized, &fields)).To(Succeed())
	g.Expect(fields).NotTo(HaveKey("spec"))
}
