package main

import (
	"strings"
	"testing"

	"github.com/pkg/errors"
	k8sCoreV1 "k8s.io/api/core/v1"
	k8sMetaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes/fake"
	k8sTesting "k8s.io/client-go/testing"
)

type tksData struct {
	systemSecretTokenAccessor   string
	systemSecretAppRoleSecretID string
}

func (d *vtkData) testK8sServer(t *testing.T) *tksData {
	t.Helper()

	k8sClient := fake.NewSimpleClientset()
	createSecretsReactor := func(action k8sTesting.Action) (handled bool, ret runtime.Object, err error) {
		s := action.(k8sTesting.CreateAction).GetObject().(*k8sCoreV1.Secret)
		if errs := validation.IsDNS1123Subdomain(s.Name); errs != nil {
			return true, nil, errors.New(strings.Join(errs, ","))
		}
		return false, nil, nil
	}
	k8sClient.PrependReactor("create", "secrets", createSecretsReactor)
	d.k8sClient = k8sClient

	// Default params
	tksd := &tksData{}
	tksd.systemSecretTokenAccessor = "fake_token-accessor"
	tksd.systemSecretAppRoleSecretID = "fake_approle_secret-id"

	return tksd
}

// Create application system k8s secret
func (d *vtkData) testK8sServerCreateSystemSecret(t *testing.T, tksd *tksData, param string) {
	t.Helper()

	s := make(map[string]interface{})
	s["token-accessor"] = tksd.systemSecretTokenAccessor
	s["approle_secret-id"] = tksd.systemSecretAppRoleSecretID
	// Convert data (should be base64 encoded in k8s)
	data := make(map[string][]byte)
	for k, v := range s {
		data[k] = []byte(v.(string))
	}

	secret := &k8sCoreV1.Secret{}
	secretName := appName + "-system"
	secret.Name = secretName
	secret.Data = data
	if param != "NoAnnotation" {
		annotations := make(map[string]string)
		createdByValue := appName
		if param == "IncorrectAnnotation" {
			createdByValue = "incorrect-annotation"
		}
		annotations["createdBy"] = createdByValue
		secret.Annotations = annotations
	}

	if _, err := d.k8sClient.CoreV1().Secrets(podNamespace).Create(secret); err != nil {
		t.Fatal(errors.Wrap(err, "Error during create application k8s secret"))
	}
}

// Create k8s secrets
func (d *vtkData) testK8sServerCreateSecret(t *testing.T, secretName, secretNamespace, secretAnnotationName, secretAnnotationValue string) {
	t.Helper()

	s := make(map[string]interface{})
	s["testK8sKey-"+secretName] = "testK8sValue-" + secretName
	// Convert data (should be base64 encoded in k8s)
	data := make(map[string][]byte)
	for k, v := range s {
		data[k] = []byte(v.(string))
	}

	secret := &k8sCoreV1.Secret{}
	secret.Name = secretName
	secret.Data = data
	annotations := make(map[string]string)
	annotations[secretAnnotationName] = secretAnnotationValue
	secret.Annotations = annotations

	if _, err := d.k8sClient.CoreV1().Secrets(secretNamespace).Create(secret); err != nil {
		t.Fatal(errors.Wrap(err, "Error during create k8s secret"))
	}
}

// Read k8s secret
func (d *vtkData) testK8sServerReadTestSecret(t *testing.T, secretName, secretNamespace string) (*k8sCoreV1.Secret, error) {
	t.Helper()

	secretData, err := d.k8sClient.CoreV1().Secrets(secretNamespace).Get(secretName, k8sMetaV1.GetOptions{})
	if err != nil {
		err = errors.Wrap(err, "Error during read k8s secret")
	}

	return secretData, err
}
