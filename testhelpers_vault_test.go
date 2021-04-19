package main

import (
	"fmt"
	"net"
	"strings"
	"testing"

	kv "github.com/hashicorp/vault-plugin-secrets-kv"
	"github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/builtin/credential/approle"
	"github.com/hashicorp/vault/http"
	"github.com/hashicorp/vault/sdk/helper/logging"
	"github.com/hashicorp/vault/sdk/logical"
	"github.com/hashicorp/vault/vault"
)

const (
	tvsMountPath          = "testMount"
	tvsAppRoleName        = "vault-to-k8s"
	tvsAppRoleTokenTTL    = 3600
	tvsAppRoleSecretIDTTL = 7200
	tvsAppRolePolicyName  = "vault-to-k8s"
	tvsAppRolePolicyRules = `# Access to secrets
path "testMount/k8s/*"  {
  capabilities = ["read", "list"]
}

# Read mounts
path "sys/mounts"  {
  capabilities = ["read"]
}

# List metadata
path "testMount/metadata/k8s/*"  {
  capabilities = ["list"]
}

# Read data
path "testMount/data/k8s/*"  {
  capabilities = ["read"]
}

# Lookup 'secret_id'
path "auth/approle/role/vault-to-k8s/secret-id/lookup"  {
  capabilities = ["update"]
}

# Create 'secret_id'
path "auth/approle/role/vault-to-k8s/secret-id"  {
  capabilities = ["update"]
}

# Revoke 'token' and 'token accessor'
path "auth/token/revoke-accessor"  {
  capabilities = ["update"]
}

# Revoke 'secret_id' and 'secret_id_accessor'
path "auth/approle/role/vault-to-k8s/secret-id-accessor/destroy"  {
  capabilities = ["update"]
}`
)

type tvsData struct {
	server      net.Listener
	secretsList []string
}

func (d *vtkData) testVaultServer(t *testing.T) *tvsData {
	t.Helper()

	var err error
	tvsd := &tvsData{}
	var addr string
	tvsDebug := false

	// Default params
	d.approleName = tvsAppRoleName
	d.nonVersioningNamespacesList = append(d.nonVersioningNamespacesList, "k8s-ns-nonver")

	// Configure Vault logger
	// See log levels: https://github.com/hashicorp/vault/blob/master/vendor/github.com/hashicorp/go-hclog/logger.go
	logger := logging.NewVaultLogger(0)
	if !tvsDebug {
		logger = logging.NewVaultLogger(6)
	}
	// Add AppRole auth backend
	coreConfig := &vault.CoreConfig{
		LogicalBackends: map[string]logical.Factory{
			"kv": kv.Factory,
		},
		CredentialBackends: map[string]logical.Factory{
			"approle": approle.Factory,
		},
		Logger: logger,
	}

	// Create an in-memory, unsealed core
	core, _, rootToken := vault.TestCoreUnsealedWithConfig(t, coreConfig)

	// Start an HTTP server for the core
	tvsd.server, addr = http.TestServer(t, core)

	// Create a client that talks to the server, initially authenticating with the root token
	d.vaultClient, err = newVaultClient(addr)
	if err != nil {
		t.Fatal(err)
	}
	d.vaultClient.SetToken(rootToken)

	// Configure server
	// Enable KV2 Secrets Engine
	optionsMount := &api.MountInput{
		Type: "kv-v2",
	}
	if err := d.vaultClient.Sys().Mount(tvsMountPath, optionsMount); err != nil {
		t.Fatal(err)
	}

	// Create policy for AppRole
	if err = d.vaultClient.Sys().PutPolicy(tvsAppRolePolicyName, tvsAppRolePolicyRules); err != nil {
		t.Fatal(err)
	}

	// Enable AppRole Auth Method
	optionsAppRoleAuth := &api.MountInput{
		Type: "approle",
	}
	if err = d.vaultClient.Sys().EnableAuthWithOptions("approle", optionsAppRoleAuth); err != nil {
		t.Fatal(err)
	}

	// Create AppRole with attached policy
	optionsAppRole := map[string]interface{}{
		"policies":      tvsAppRolePolicyName,
		"token_ttl":     tvsAppRoleTokenTTL,
		"secret_id_ttl": tvsAppRoleSecretIDTTL,
	}
	tvsAppRolePath := fmt.Sprintf("auth/approle/role/%s", tvsAppRoleName)
	if _, err = d.vaultClient.Logical().Write(tvsAppRolePath, optionsAppRole); err != nil {
		t.Fatal(err)
	}

	// Fetch Role ID
	tvsAppRoleRoleIDPath := fmt.Sprintf("auth/approle/role/%s/role-id", tvsAppRoleName)
	getTvsAppRoleRoleID, err := d.vaultClient.Logical().Read(tvsAppRoleRoleIDPath)
	if err != nil {
		t.Fatal(err)
	}
	approleRoleID = fmt.Sprintf("%s", getTvsAppRoleRoleID.Data["role_id"])

	// Create default secrets
	tvsd.secretsList = []string{
		"secret1",
		"secret2." + k8sClusterName,
		"secret3.another-" + k8sClusterName,
		"secret4.something." + k8sClusterName,
		"secret5." + k8sClusterName + ".something",
		"secret6",
	}
	d.testVaultServerCreateSecrets(t, tvsd.secretsList, "k8s-ns1")
	// Create "secret1" version 2
	d.testVaultServerCreateSecrets(t, []string{"secret1"}, "k8s-ns1")
	// Create "secret10" in "k8s-ns2" namespace
	d.testVaultServerCreateSecrets(t, []string{"secret10"}, "k8s-ns2")

	// Server debug
	if tvsDebug {
		testVaultServerDebug(d, tvsd)
	}

	return tvsd
}

func testVaultServerDebug(d *vtkData, tvsd *tvsData) {
	fmt.Println()
	fmt.Println("Test Vault server data:")
	// Verify mounts
	tvsMountPaths, err := d.vaultClient.Sys().ListMounts()
	if err != nil {
		fmt.Println("Error:", err)
	}
	fmt.Println()
	fmt.Println("Mount paths:")
	fmt.Println(tvsMountPaths)
	fmt.Println()
	fmt.Println("Mount paths with engine type and version:")
	for k, m := range tvsMountPaths {
		fmt.Printf("Mount '%s' has engine type '%s' and version '%s'", k, m.Type, m.Options["version"])
		fmt.Println()
	}

	// List policies
	tvsPolicies, err := d.vaultClient.Sys().ListPolicies()
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println()
	fmt.Println("Policies:")
	fmt.Println(tvsPolicies)

	// Get AppRole policy
	getTvsAppRolePolicy, err := d.vaultClient.Sys().GetPolicy(tvsAppRolePolicyName)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println()
	fmt.Println("AppRole policy '" + tvsAppRolePolicyName + "' rules:")
	fmt.Println(getTvsAppRolePolicy)

	// List Auth Methods
	tvsAuthMethods, err := d.vaultClient.Sys().ListAuth()
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println()
	fmt.Println("Enabled Auth methods:")
	fmt.Println(tvsAuthMethods)

	// Get AppRole
	tvsAppRolePath := fmt.Sprintf("auth/approle/role/%s", tvsAppRoleName)
	getTvsAppRole, err := d.vaultClient.Logical().Read(tvsAppRolePath)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println()
	fmt.Println("Get AppRole '" + tvsAppRoleName + "':")
	fmt.Println(getTvsAppRole)

	// AppRole Role ID
	fmt.Println()
	fmt.Println("AppRole Role ID:")
	fmt.Println(approleRoleID)

	// List secrets
	tvsVaultMount := strings.SplitN(vaultSecretsPath, "/", 2)[0]
	tvsVaultSecretsMount := strings.SplitN(vaultSecretsPath, "/", 2)[1]
	tvsMountPath := tvsVaultMount + "/metadata/" + tvsVaultSecretsMount
	// In 'k8s-ns1' NS
	tvsListSecretsNS1, err := d.vaultClient.Logical().List(tvsMountPath + "/k8s-ns1")
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println()
	fmt.Println("Secrets under '" + vaultSecretsPath + "/k8s-ns1'")
	fmt.Println(tvsListSecretsNS1)

	// In 'k8s-ns2' NS
	tvsListSecretsNS2, err := d.vaultClient.Logical().List(tvsMountPath + "/k8s-ns2")
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println()
	fmt.Println("Secrets under '" + vaultSecretsPath + "/k8s-ns2'")
	fmt.Println(tvsListSecretsNS2)

	fmt.Println()
}

// Fetch token
func (d *vtkData) testVaultServerFetchToken(t *testing.T) {
	t.Helper()

	// Create Secret ID
	d.testVaultServerCreateSecretID(t)

	// Params for fetch token
	options := map[string]interface{}{
		"role_id":   approleRoleID,
		"secret_id": d.approleSecretID,
	}
	authPath := fmt.Sprintf("auth/%s/login", authMethod)

	// Fetching token
	vaultTokenValues, err := d.vaultClient.Logical().Write(authPath, options)
	if err != nil {
		t.Fatal(err)
	}
	d.vaultTokenAccessor = vaultTokenValues.Auth.Accessor
}

// Create Secret ID
func (d *vtkData) testVaultServerCreateSecretID(t *testing.T) {
	t.Helper()

	tvsAppRoleSecretIDPath := fmt.Sprintf("auth/approle/role/%s/secret-id", tvsAppRoleName)
	tvsAppRoleSecretID, err := d.vaultClient.Logical().Write(tvsAppRoleSecretIDPath, map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	d.approleSecretID = tvsAppRoleSecretID.Data["secret_id"]
}

// Create Wrapped Secret ID with 1m ttl
func (d *vtkData) testVaultServerCreateWrappedSecretID(t *testing.T) {
	t.Helper()

	d.vaultClient.SetWrappingLookupFunc(func(operation, path string) string {
		return "1m"
	})
	defer d.vaultClient.SetWrappingLookupFunc(nil)

	tvsAppRoleWrappedSecretIDPath := fmt.Sprintf("auth/approle/role/%s/secret-id", tvsAppRoleName)
	tvsAppRoleWrappedSecretID, err := d.vaultClient.Logical().Write(tvsAppRoleWrappedSecretIDPath, map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	approleSecretIDWrappedToken = fmt.Sprintf("%s", tvsAppRoleWrappedSecretID.WrapInfo.Token)
}

// Create secrets
func (d *vtkData) testVaultServerCreateSecrets(t *testing.T, secretsList []string, secretNamespace string) {
	t.Helper()

	tvsVaultMount := strings.SplitN(vaultSecretsPath, "/", 2)[0]
	tvsVaultSecretsMount := strings.SplitN(vaultSecretsPath, "/", 2)[1]
	tvsMountPath := tvsVaultMount + "/data/" + tvsVaultSecretsMount

	for _, secretName := range secretsList {
		optionsSecret := map[string]interface{}{
			"data": map[string]interface{}{
				"testKey-" + secretName: "testValue-" + secretName,
			},
		}
		vaultSecretPathFull := tvsMountPath + "/" + secretNamespace + "/" + secretName
		if _, err := d.vaultClient.Logical().Write(vaultSecretPathFull, optionsSecret); err != nil {
			t.Fatal(err)
		}
	}
}

// Create secrets with incorrect data
func (d *vtkData) testVaultServerCreateSecretsIncorrectData(t *testing.T, secretsList []string, secretNamespace string) {
	t.Helper()

	tvsVaultMount := strings.SplitN(vaultSecretsPath, "/", 2)[0]
	tvsVaultSecretsMount := strings.SplitN(vaultSecretsPath, "/", 2)[1]
	tvsMountPath := tvsVaultMount + "/data/" + tvsVaultSecretsMount

	for _, secretName := range secretsList {
		optionsSecret := map[string]interface{}{
			"data": map[string]interface{}{
				"testKey-" + secretName: []string{"some_data"},
			},
		}
		vaultSecretPathFull := tvsMountPath + "/" + secretNamespace + "/" + secretName
		if _, err := d.vaultClient.Logical().Write(vaultSecretPathFull, optionsSecret); err != nil {
			t.Fatal(err)
		}
	}
}

// Delete secret
func (d *vtkData) testVaultServerDeleteSecret(t *testing.T, secretName, secretNamespace string) {
	t.Helper()

	tvsVaultMount := strings.SplitN(vaultSecretsPath, "/", 2)[0]
	tvsVaultSecretsMount := strings.SplitN(vaultSecretsPath, "/", 2)[1]
	tvsMountPath := tvsVaultMount + "/data/" + tvsVaultSecretsMount

	vaultSecretPathFull := tvsMountPath + "/" + secretNamespace + "/" + secretName
	if _, err := d.vaultClient.Logical().Delete(vaultSecretPathFull); err != nil {
		t.Fatal(err)
	}
}
