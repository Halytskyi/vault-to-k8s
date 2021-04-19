package main

import (
	"fmt"
	"strings"
	"testing"

	k8sMetaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Authenticate in Vault: error during read application system k8s secret
func TestApproleAuthenticateErrorK8sReadSecret(t *testing.T) {
	d := &vtkData{}
	tvsd := d.testVaultServer(t)
	defer tvsd.server.Close()
	_ = d.testK8sServer(t)

	err := d.approleAuthenticate()
	if err == nil {
		t.Fatal("Expected error, but it wasn't returned")
	}

	if !strings.Contains(err.Error(), "-system\" not found") {
		t.Log(err)
		t.Fatal("Incorrect error response")
	}
}

// Authenticate in Vault with invalid Secret ID
func TestApproleAuthenticateInvalidSecretID(t *testing.T) {
	d := &vtkData{}
	tvsd := d.testVaultServer(t)
	defer tvsd.server.Close()
	tksd := d.testK8sServer(t)
	d.testK8sServerCreateSystemSecret(t, tksd, "none")

	err := d.approleAuthenticate()
	if err == nil {
		t.Fatal("Expected error, but it wasn't returned")
	}

	if !strings.Contains(err.Error(), "invalid secret id") {
		t.Log(err)
		t.Fatal("Incorrect error response")
	}
}

// Authenticate in Vault: error during update application k8s secret
func TestApproleAuthenticateNoAnnotation(t *testing.T) {
	d := &vtkData{}
	tvsd := d.testVaultServer(t)
	defer tvsd.server.Close()
	tksd := d.testK8sServer(t)
	d.testK8sServerCreateSystemSecret(t, tksd, "NoAnnotation")
	d.testVaultServerCreateWrappedSecretID(t)

	err := d.approleAuthenticate()
	if err == nil {
		t.Fatal("Expected error, but it wasn't returned")
	}

	if !strings.Contains(err.Error(), "Can't update application k8s secret") {
		t.Log(err)
		t.Fatal("Incorrect error response")
	}
}

// Authenticate in Vault
func TestApproleAuthenticate(t *testing.T) {
	d := &vtkData{}
	tvsd := d.testVaultServer(t)
	defer tvsd.server.Close()
	tksd := d.testK8sServer(t)
	d.testK8sServerCreateSystemSecret(t, tksd, "none")
	d.testVaultServerCreateWrappedSecretID(t)

	if err := d.approleAuthenticate(); err != nil {
		t.Log(err)
		t.Fatal("Error should not be raised")
	}
}

// Test get token with invalid Wrapped Secret ID Token
func TestApproleGetTokenInvalidWrappedSecretIDToken(t *testing.T) {
	d := &vtkData{}
	tvsd := d.testVaultServer(t)
	defer tvsd.server.Close()

	approleSecretIDWrappedToken = "fake"

	err := d.approleGetToken()
	if err == nil {
		t.Fatal("Expected error, but it wasn't returned")
	}

	if !strings.Contains(err.Error(), "There is no valid 'wrapped token'") {
		t.Log(err)
		t.Fatal("Incorrect error response")
	}
}

// Test get token with invalid Secret ID
func TestApproleGetTokenInvalidSecretID(t *testing.T) {
	d := &vtkData{}
	tvsd := d.testVaultServer(t)
	defer tvsd.server.Close()

	d.approleSecretID = "fake"

	err := d.approleGetToken()
	if err == nil {
		t.Fatal("Expected error, but it wasn't returned")
	}

	if !strings.Contains(err.Error(), "invalid secret id") {
		t.Log(err)
		t.Fatal("Incorrect error response")
	}
}

// Test get token with default 'TOKEN_ROTATION_INTERVAL' value
func TestApproleGetToken(t *testing.T) {
	d := &vtkData{}
	tvsd := d.testVaultServer(t)
	defer tvsd.server.Close()
	d.testVaultServerCreateWrappedSecretID(t)

	tokenRotationInterval = -1 // default value

	if err := d.approleGetToken(); err != nil {
		t.Log(err)
		t.Fatal("Error should not be raised")
	}

	if d.approleName != "vault-to-k8s" {
		t.Fatalf("Incorrect value for app_role: '%s'. Expected 'vault-to-k8s'", d.approleName)
	}
	if d.vaultTokenTTL["ttl"] != int64(3600) {
		t.Fatalf("Incorrect value for lease_duration: '%d'. Expected '3600'", d.vaultTokenTTL["ttl"])
	}
	if tokenRotationInterval != 2520 {
		t.Fatalf("Incorrect value for tokenRotationInterval: '%d'. Expected '2520'", tokenRotationInterval)
	}
}

// Test get token with value of 'TOKEN_ROTATION_INTERVAL' more than 'lease_duration'
func TestApproleGetTokenBigTokenRotationInterval(t *testing.T) {
	d := &vtkData{}
	tvsd := d.testVaultServer(t)
	defer tvsd.server.Close()
	d.testVaultServerCreateWrappedSecretID(t)

	tokenRotationInterval = 3601

	if err := d.approleGetToken(); err != nil {
		t.Log(err)
		t.Fatal("Error should not be raised")
	}

	if tokenRotationInterval != 2520 {
		t.Fatalf("Incorrect value for tokenRotationInterval: '%d'. Expected '2520'", tokenRotationInterval)
	}
}

// Test read application system k8s secret
func TestApproleReadAppSecret(t *testing.T) {
	d := &vtkData{}
	tksd := d.testK8sServer(t)
	d.testK8sServerCreateSystemSecret(t, tksd, "none")

	if err := d.approleReadAppSecret(); err != nil {
		t.Log(err)
		t.Fatal("Error should not be raised")
	}

	if d.approleSecretID != tksd.systemSecretAppRoleSecretID {
		t.Fatalf("Incorrect value for d.approleSecretID: '%s'. Expected '%s'", d.approleSecretID, tksd.systemSecretAppRoleSecretID)
	}
}

// Test create application system secret
func TestUpdateAppSystemSecretCreate(t *testing.T) {
	d := &vtkData{}
	d.testK8sServer(t)

	d.vaultTokenAccessor = "new_fake_token-accessor"
	d.approleSecretID = "new_fake_approle_secret-id"

	if err := d.updateAppSystemSecret(); err != nil {
		t.Log(err)
		t.Fatal("Error should not be raised")
	}

	appSecret, err := d.k8sClient.CoreV1().Secrets(podNamespace).Get(appName+"-system", k8sMetaV1.GetOptions{})
	if err != nil {
		t.Log(err)
		t.Fatal("Error should not be raised")
	}

	k8sSystemSecretTokenAccessor := string([]byte(appSecret.Data["token-accessor"]))
	k8sSystemSecretAppRoleSecretID := string([]byte(appSecret.Data["approle_secret-id"]))
	k8sSystemSecretAnnotationCreatedBy := appSecret.Annotations["createdBy"]

	if d.vaultTokenAccessor != k8sSystemSecretTokenAccessor {
		t.Fatalf("Incorrect value for secret key 'token-accessor': '%s'. Expected '%s'", k8sSystemSecretTokenAccessor, d.vaultTokenAccessor)
	}
	if d.approleSecretID != k8sSystemSecretAppRoleSecretID {
		t.Fatalf("Incorrect value for secret key 'approle_secret-id': '%s'. Expected '%s'", k8sSystemSecretAppRoleSecretID, d.approleSecretID)
	}
	if appName != k8sSystemSecretAnnotationCreatedBy {
		t.Fatalf("Incorrect value for secret annotation 'createdBy': '%s'. Expected '%s'", k8sSystemSecretAnnotationCreatedBy, appName)
	}
}

// Test update application system secret
func TestUpdateAppSystemSecretUpdate(t *testing.T) {
	d := &vtkData{}
	tksd := d.testK8sServer(t)
	d.testK8sServerCreateSystemSecret(t, tksd, "none")

	d.vaultTokenAccessor = "new_fake_token-accessor"
	d.approleSecretID = "new_fake_approle_secret-id"

	if err := d.updateAppSystemSecret(); err != nil {
		t.Log(err)
		t.Fatal("Error should not be raised")
	}

	appSecret, err := d.k8sClient.CoreV1().Secrets(podNamespace).Get(appName+"-system", k8sMetaV1.GetOptions{})
	if err != nil {
		t.Log(err)
		t.Fatal("Error should not be raised")
	}

	k8sSystemSecretTokenAccessor := string([]byte(appSecret.Data["token-accessor"]))
	k8sSystemSecretAppRoleSecretID := string([]byte(appSecret.Data["approle_secret-id"]))
	k8sSystemSecretAnnotationCreatedBy := appSecret.Annotations["createdBy"]

	if d.vaultTokenAccessor != k8sSystemSecretTokenAccessor {
		t.Fatalf("Incorrect value for secret key 'token-accessor': '%s'. Expected '%s'", k8sSystemSecretTokenAccessor, d.vaultTokenAccessor)
	}
	if d.approleSecretID != k8sSystemSecretAppRoleSecretID {
		t.Fatalf("Incorrect value for secret key 'approle_secret-id': '%s'. Expected '%s'", k8sSystemSecretAppRoleSecretID, d.approleSecretID)
	}
	if appName != k8sSystemSecretAnnotationCreatedBy {
		t.Fatalf("Incorrect value for secret annotation 'createdBy': '%s'. Expected '%s'", k8sSystemSecretAnnotationCreatedBy, appName)
	}
}

// Test update application system secret which exists but doesn't have application annotation
func TestUpdateAppSystemSecretExistsWithoutAnnotation(t *testing.T) {
	d := &vtkData{}
	tksd := d.testK8sServer(t)
	d.testK8sServerCreateSystemSecret(t, tksd, "NoAnnotation")

	d.vaultTokenAccessor = "new_fake_token-accessor"
	d.approleSecretID = "new_fake_approle_secret-id"

	err := d.updateAppSystemSecret()
	if err == nil {
		t.Fatal("Expected error, but it wasn't returned")
	}

	if !strings.Contains(err.Error(), "Can't update application k8s secret") {
		t.Log(err)
		t.Fatal("Incorrect error response")
	}
}

// Test update application system secret which exists but with incorrect application annotation
func TestUpdateAppSystemSecretExistsIncorrectAnnotation(t *testing.T) {
	d := &vtkData{}
	tksd := d.testK8sServer(t)
	d.testK8sServerCreateSystemSecret(t, tksd, "IncorrectAnnotation")

	d.vaultTokenAccessor = "new_fake_token-accessor"
	d.approleSecretID = "new_fake_approle_secret-id"

	err := d.updateAppSystemSecret()
	if err == nil {
		t.Fatal("Expected error, but it wasn't returned")
	}

	if !strings.Contains(err.Error(), "but it wasn't created by this application") {
		t.Log(err)
		t.Fatal("Incorrect error response")
	}
}

// Test revoke old AppRole Secret ID from Vault which was expired (invalid)
func TestApproleRevokeOldSecretIDInvalid(t *testing.T) {
	d := &vtkData{}
	tvsd := d.testVaultServer(t)
	defer tvsd.server.Close()
	tksd := d.testK8sServer(t)
	d.testK8sServerCreateSystemSecret(t, tksd, "none")

	if err := d.approleRevokeOldSecretID(); err != nil {
		t.Log(err)
		t.Fatal("Error should not be raised")
	}
}

// Test revoke old AppRole Secret ID from Vault
func TestApproleRevokeOldSecretID(t *testing.T) {
	d := &vtkData{}
	tvsd := d.testVaultServer(t)
	defer tvsd.server.Close()
	d.testVaultServerCreateSecretID(t)
	tksd := d.testK8sServer(t)
	tksd.systemSecretAppRoleSecretID = fmt.Sprintf("%s", d.approleSecretID)
	d.testK8sServerCreateSystemSecret(t, tksd, "none")

	if err := d.approleRevokeOldSecretID(); err != nil {
		t.Log(err)
		t.Fatal("Error should not be raised")
	}
}

// Test revoke old Token from Vault which not exists in k8s secret (invalid)
func TestApproleRevokeOldTokenInvalid(t *testing.T) {
	d := &vtkData{}
	tksd := d.testK8sServer(t)
	tksd.systemSecretTokenAccessor = ""
	d.testK8sServerCreateSystemSecret(t, tksd, "none")

	if err := d.revokeOldToken(); err != nil {
		t.Log(err)
		t.Fatal("Error should not be raised")
	}
}

// Test revoke old Token from Vault
func TestApproleRevokeOldToken(t *testing.T) {
	d := &vtkData{}
	tvsd := d.testVaultServer(t)
	defer tvsd.server.Close()
	tksd := d.testK8sServer(t)
	d.testVaultServerFetchToken(t)
	tksd.systemSecretTokenAccessor = d.vaultTokenAccessor
	d.testK8sServerCreateSystemSecret(t, tksd, "none")

	if err := d.revokeOldToken(); err != nil {
		t.Log(err)
		t.Fatal("Error should not be raised")
	}
}

// Test AppRole SecretID lookup
func TestApproleSecretIDLookup(t *testing.T) {
	d := &vtkData{}
	tvsd := d.testVaultServer(t)
	defer tvsd.server.Close()
	d.testVaultServerCreateSecretID(t)

	if err := d.approleSecretIDLookup(); err != nil {
		t.Log(err)
		t.Fatal("Error should not be raised")
	}

	if d.approleSecretIDTTL["secret_id_ttl"] != tvsAppRoleSecretIDTTL {
		t.Fatalf("Incorrect value for 'secret_id_ttl': '%d'. Expected '%d'", d.approleSecretIDTTL["secret_id_ttl"], tvsAppRoleSecretIDTTL)
	}
}
