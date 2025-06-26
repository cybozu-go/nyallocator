package e2e

import (
	"bytes"
	"fmt"
	"os/exec"
	"testing"
	"time"

	nyallocatorv1 "github.com/cybozu-go/nyallocator/api/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)
	SetDefaultEventuallyTimeout(30 * time.Second)
	SetDefaultEventuallyPollingInterval(100 * time.Millisecond)
	RunSpecs(t, "E2e Suite")
}

var _ = BeforeSuite(func() {

})

func kubectl(input []byte, args ...string) ([]byte, []byte, error) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := exec.Command("./bin/kubectl", args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}
	err := cmd.Run()
	if err != nil {
		return stdout.Bytes(), stderr.Bytes(), fmt.Errorf("kubectl failed with %s:", err)
	}
	return stdout.Bytes(), stderr.Bytes(), nil
}

func isSufficient(nodeTemplate *nyallocatorv1.NodeTemplate) bool {
	for _, condition := range nodeTemplate.Status.Conditions {
		if condition.Type == "Sufficient" && condition.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}
