package main

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
)

// Define application init params
func defineAppInitParams() {
	vaultAddr = "test.vaul.url"
	vaultNamespace = "vault-ns"
	appName = "vault-to-k8s"
	podNamespace = "k8s-ns"
	authMethod = "approle"
	k8sClusterName = "k8s-cluster"
	vaultSecretsPath = "testMount/k8s/dev"
	annotationName = "vault-to-k8s/secret"
}

// Run before start testing
func init() {
	defineAppInitParams()
}

// Check if item in array
func checkItemInArray(arr []string, str string) bool {
	for _, item := range arr {
		if item == str {
			return true
		}
	}
	return false
}

// Test get 'string' environment variable with defined ENV variable
func TestGetEnvWithDefaultStringDefinedEnv(t *testing.T) {
	os.Setenv("PARAM1", "test")
	result := getEnvWithDefaultString("PARAM1", "def-value")
	if result != "test" {
		t.Fatalf("Incorrect value '%s', expected 'test'", result)
	}
}

// Test get 'string' environment variable without defined ENV variable, return default value
func TestGetEnvWithDefaultStringWithoutDefinedEnv(t *testing.T) {
	result := getEnvWithDefaultString("PARAM2", "def-value")
	if result != "def-value" {
		t.Fatalf("Incorrect value '%s', expected 'def-value'", result)
	}
}

// Test get 'int' environment variable with defined ENV variable
func TestGetEnvWithDefaultIntDefinedEnv(t *testing.T) {
	os.Setenv("PARAM3", "5")
	result := getEnvWithDefaultInt("PARAM3", 9)
	if result != 5 {
		t.Fatalf("Incorrect value '%d', expected '5'", result)
	}
}

// Test get 'int' environment variable without defined ENV variable, return default value
func TestGetEnvWithDefaultIntWithoutDefinedEnv(t *testing.T) {
	result := getEnvWithDefaultInt("PARAM4", 9)
	if result != 9 {
		t.Fatalf("Incorrect value '%d', expected '9'", result)
	}
}

// Test get pod namespace variable with defined ENV variable
func TestGetEnvPodNamespaceDefinedEnv(t *testing.T) {
	os.Setenv("POD_NAMESPACE", "my-k8s-ns")
	result := getEnvPodNamespace()
	if result != "my-k8s-ns" {
		t.Fatalf("Incorrect value '%s', expected 'my-k8s-ns'", result)
	}

	// Clean ENV variable
	os.Unsetenv("POD_NAMESPACE")
}

// Test verify config parameters
func TestVerifyConfig(t *testing.T) {
	if err := verifyConfig(); err != nil {
		t.Log(err)
		t.Fatal("Error should not be raised")
	}
}

// Test verify if mount exists in Vault and has correct engine version: wrong mount path
func TestVerifyVaultMountWrongMountPath(t *testing.T) {
	d := &vtkData{}
	tvsd := d.testVaultServer(t)
	defer tvsd.server.Close()
	vaultSecretsPath = "testMount2/k8s/dev"

	err := d.verifyVaultMount()
	// Re-init default app params
	defineAppInitParams()
	if err == nil {
		t.Fatal("Expected error, but it wasn't returned")
	}

	if !strings.Contains(err.Error(), "doesn't exists in Vault") {
		t.Log(err)
		t.Fatal("Incorrect error response")
	}
}

// Test verify if mount exists in Vault and has correct engine version: wrong engine type
func TestVerifyVaultMountWrongEngineType(t *testing.T) {
	d := &vtkData{}
	tvsd := d.testVaultServer(t)
	defer tvsd.server.Close()
	vaultSecretsPath = "cubbyhole/k8s/dev"

	err := d.verifyVaultMount()
	// Re-init default app params
	defineAppInitParams()
	if err == nil {
		t.Fatal("Expected error, but it wasn't returned")
	}

	if !strings.Contains(err.Error(), "is not of type kv") {
		t.Log(err)
		t.Fatal("Incorrect error response")
	}
}

// Test verify if mount exists in Vault and has correct engine version: wrong engine version
func TestVerifyVaultMountWrongEngineVersion(t *testing.T) {
	d := &vtkData{}
	tvsd := d.testVaultServer(t)
	defer tvsd.server.Close()
	vaultSecretsPath = "secret/k8s/dev"

	err := d.verifyVaultMount()
	// Re-init default app params
	defineAppInitParams()
	if err == nil {
		t.Fatal("Expected error, but it wasn't returned")
	}

	if !strings.Contains(err.Error(), "Vault mount version is not '2'") {
		t.Log(err)
		t.Fatal("Incorrect error response")
	}
}

// Test verify if mount exists in Vault and has correct engine version
func TestVerifyVaultMount(t *testing.T) {
	d := &vtkData{}
	tvsd := d.testVaultServer(t)
	defer tvsd.server.Close()

	if err := d.verifyVaultMount(); err != nil {
		t.Log(err)
		t.Fatal("Error should not be raised")
	}
}

// Test list namespaces from Vault with wrong path
func TestNamespacesListWrongPath(t *testing.T) {
	d := &vtkData{}
	tvsd := d.testVaultServer(t)
	defer tvsd.server.Close()
	vaultSecretsPath = "testMount/k8s/dev-wrong"

	listNamespaces, err := d.vaultNamespacesList()
	// Re-init default app params
	defineAppInitParams()
	if err != nil {
		t.Log(err)
		t.Fatal("Error should not be raised")
	}
	if listNamespaces != nil {
		t.Fatalf("Incorrect value '%s', expected 'nil'", listNamespaces)
	}
}

// Test list namespaces from Vault
func TestNamespacesList(t *testing.T) {
	d := &vtkData{}
	tvsd := d.testVaultServer(t)
	defer tvsd.server.Close()

	listNamespaces, err := d.vaultNamespacesList()
	if err != nil {
		t.Log(err)
		t.Fatal("Error should not be raised")
	}

	if result := checkItemInArray(listNamespaces, "k8s-ns1"); result != true {
		t.Fatalf("Namespace 'k8s-ns1' didn't find in response")
	}
	if result := checkItemInArray(listNamespaces, "k8s-ns2"); result != true {
		t.Fatalf("Namespace 'k8s-ns2' didn't find in response")
	}
}

// Test list secrets from Vault with wrong namespace
func TestSecretsListWrongNamespace(t *testing.T) {
	d := &vtkData{}
	tvsd := d.testVaultServer(t)
	defer tvsd.server.Close()

	listSecrets, err := d.secretsList("k8s-ns1-wrong")
	if err != nil {
		t.Log(err)
		t.Fatal("Error should not be raised")
	}
	if listSecrets != nil {
		t.Fatalf("Incorrect value '%s', expected 'nil'", listSecrets)
	}
}

// Test list secrets from Vault
func TestSecretsList(t *testing.T) {
	d := &vtkData{}
	tvsd := d.testVaultServer(t)
	defer tvsd.server.Close()

	listSecrets, err := d.secretsList("k8s-ns1")
	if err != nil {
		t.Log(err)
		t.Fatal("Error should not be raised")
	}

	if result := checkItemInArray(listSecrets, "secret1"); result != true {
		t.Fatalf("Secret 'secret1' didn't find in response")
	}
	if result := checkItemInArray(listSecrets, "secret2."+k8sClusterName); result != true {
		t.Fatalf("Secret 'secret2.%s' didn't find in response", k8sClusterName)
	}
}

// Test read secrets from Vault with wrong secret name
func TestSecretsReadWrongSecret(t *testing.T) {
	d := &vtkData{}
	tvsd := d.testVaultServer(t)
	defer tvsd.server.Close()

	secretData, secretVersion, err := d.secretsRead(vaultSecretsPath + "/k8s-ns1/secret1-wrong")
	if err != nil {
		t.Log(err)
		t.Fatal("Error should not be raised")
	}

	if len(secretData) != 0 {
		t.Fatalf("Incorrect value '%d', expected '0'", len(secretData))
	}
	if secretVersion != "" {
		t.Fatalf("Incorrect value '%s', expected ''", secretVersion)
	}
}

// Test read secrets from Vault
func TestSecretsRead(t *testing.T) {
	d := &vtkData{}
	tvsd := d.testVaultServer(t)
	defer tvsd.server.Close()

	secretData, secretVersion, err := d.secretsRead(vaultSecretsPath + "/k8s-ns1/secret1")
	if err != nil {
		t.Log(err)
		t.Fatal("Error should not be raised")
	}

	if secretData["testKey-secret1"] != "testValue-secret1" {
		t.Fatalf("Incorrect value '%s', expected 'testValue-secret1'", secretData["testKey-secret1"])
	}
	if secretVersion != "2" {
		t.Fatalf("Incorrect value '%s', expected '2'", secretVersion)
	}
}

// Test generate secrets list for versioning namespace
func TestFilterSecretsVersioningNS(t *testing.T) {
	d := &vtkData{}
	tvsd := d.testVaultServer(t)
	defer tvsd.server.Close()

	clusterSecrets := d.filterSecrets(tvsd.secretsList, "."+k8sClusterName, "k8s-ns-ver")

	if _, ok := clusterSecrets[tvsd.secretsList[0]]; ok {
		if clusterSecrets[tvsd.secretsList[0]] != 1 {
			t.Fatalf("Secret '%s' is non-versioning, but should be versioning", tvsd.secretsList[0])
		}
	} else {
		t.Fatalf("Secret '%s' didn't find in response", tvsd.secretsList[0])
	}
	if _, ok := clusterSecrets[tvsd.secretsList[1]]; ok {
		if clusterSecrets[tvsd.secretsList[1]] != 1 {
			t.Fatalf("Secret '%s' is non-versioning, but should be versioning", tvsd.secretsList[1])
		}
	} else {
		t.Fatalf("Secret '%s' didn't find in response", tvsd.secretsList[1])
	}
	if _, ok := clusterSecrets[tvsd.secretsList[2]]; ok {
		t.Fatalf("Secret '%s' found in response", tvsd.secretsList[2])
	}
	if _, ok := clusterSecrets[tvsd.secretsList[3]]; ok {
		t.Fatalf("Secret '%s' found in response", tvsd.secretsList[3])
	}
	if _, ok := clusterSecrets[tvsd.secretsList[4]]; ok {
		t.Fatalf("Secret '%s' found in response", tvsd.secretsList[4])
	}
	if _, ok := clusterSecrets[tvsd.secretsList[5]]; ok {
		if clusterSecrets[tvsd.secretsList[5]] != 1 {
			t.Fatalf("Secret '%s' is non-versioning, but should be versioning", tvsd.secretsList[5])
		}
	} else {
		t.Fatalf("Secret '%s' didn't find in response", tvsd.secretsList[5])
	}
}

// Test generate secrets list for non-versioning namespace
func TestFilterSecretsNonVersioningNS(t *testing.T) {
	d := &vtkData{}
	tvsd := d.testVaultServer(t)
	defer tvsd.server.Close()

	clusterSecrets := d.filterSecrets(tvsd.secretsList, "."+k8sClusterName, "k8s-ns-nonver")

	if _, ok := clusterSecrets[tvsd.secretsList[0]]; ok {
		if clusterSecrets[tvsd.secretsList[0]] != 1 {
			t.Fatalf("Secret '%s' is non-versioning, but should be versioning", tvsd.secretsList[0])
		}
	} else {
		t.Fatalf("Secret '%s' didn't find in response", tvsd.secretsList[0])
	}
	if _, ok := clusterSecrets[tvsd.secretsList[1]]; ok {
		if clusterSecrets[tvsd.secretsList[1]] != 0 {
			t.Fatalf("Secret '%s' is non-versioning, but should be versioning", tvsd.secretsList[1])
		}
	} else {
		t.Fatalf("Secret '%s' didn't find in response", tvsd.secretsList[1])
	}
	if _, ok := clusterSecrets[tvsd.secretsList[2]]; ok {
		t.Fatalf("Secret '%s' found in response", tvsd.secretsList[2])
	}
	if _, ok := clusterSecrets[tvsd.secretsList[3]]; ok {
		if clusterSecrets[tvsd.secretsList[3]] != 0 {
			t.Fatalf("Secret '%s' is non-versioning, but should be versioning", tvsd.secretsList[3])
		}
	} else {
		t.Fatalf("Secret '%s' found in response", tvsd.secretsList[3])
	}
	if _, ok := clusterSecrets[tvsd.secretsList[4]]; ok {
		t.Fatalf("Secret '%s' found in response", tvsd.secretsList[4])
	}
	if _, ok := clusterSecrets[tvsd.secretsList[5]]; ok {
		if clusterSecrets[tvsd.secretsList[5]] != 1 {
			t.Fatalf("Secret '%s' is non-versioning, but should be versioning", tvsd.secretsList[5])
		}
	} else {
		t.Fatalf("Secret '%s' didn't find in response", tvsd.secretsList[5])
	}
}

// Test create/update secrets in k8s for versioning namespace
func TestUpdateSecretsInK8sVersioningNS(t *testing.T) {
	d := &vtkData{}
	tvsd := d.testVaultServer(t)
	defer tvsd.server.Close()
	_ = d.testK8sServer(t)

	// Create secret2 in k8s with wrong annotation
	d.testK8sServerCreateSecret(t, tvsd.secretsList[1]+"-v1", "k8s-ns1", "annotation-name", "annotation-value")
	// Create secret3 in k8s with correct annotation name but wrong value
	d.testK8sServerCreateSecret(t, tvsd.secretsList[2]+"-v1", "k8s-ns1", annotationName, "annotation-value")
	// Create secret4 in k8s with correct annotation
	d.testK8sServerCreateSecret(t, tvsd.secretsList[3]+"-v1", "k8s-ns1", annotationName, vaultSecretsPath+"/k8s-ns1/"+tvsd.secretsList[3])
	// Create secret5 in k8s with correct annotation
	d.testK8sServerCreateSecret(t, tvsd.secretsList[4]+"-v1", "k8s-ns1", annotationName, vaultSecretsPath+"/k8s-ns1/"+tvsd.secretsList[4])
	// Delete secret5 data from Vault
	d.testVaultServerDeleteSecret(t, tvsd.secretsList[4], "k8s-ns1")

	// Get secrets for versioning NS
	filteredSecrets := d.filterSecrets(tvsd.secretsList, "."+k8sClusterName, "k8s-ns-ver")

	// Get list of k8s secrets
	k8sSecrets, err := d.k8sSecretsList("k8s-ns1")
	if err != nil {
		t.Log(err)
		t.Fatal("Error should not be raised")
	}

	// Test create/update secrets in k8s
	numWorkers = 1
	usjc := make(chan secretForUpdate, numWorkers)
	usrc := make(chan updateSecretResults, numWorkers)

	// WaitGroup is used to wait for the program to finish goroutines
	var wg sync.WaitGroup
	// Use context for cancelation signal in goroutines if errors occurs
	_, cancel := context.WithCancel(context.Background())
	defer cancel() // Make sure it's called to release resources even if no errors

	// Create goroutines
	wg.Add(numWorkers)
	for w := 1; w <= numWorkers; w++ {
		go d.updateSecretsInK8s(cancel, w, &wg, usjc, usrc, "k8s-ns1", "."+k8sClusterName, k8sSecrets)
	}

	// Send secrets to goroutines
	go func() {
		for filteredSecret, versioning := range filteredSecrets {
			usjc <- secretForUpdate{name: filteredSecret, versioning: versioning}
		}
		close(usjc)
	}()

	for i := 1; i <= len(filteredSecrets); i++ {
		usrcResult := <-usrc
		if usrcResult.err != nil {
			wg.Wait()
			t.Log(usrcResult.err)
			t.Fatal("Error should not be raised")
		}
	}
	close(usrc)

	// Secret1 should be created in k8s by this tool with version 2
	secret1, err := d.testK8sServerReadTestSecret(t, tvsd.secretsList[0]+"-v2", "k8s-ns1")
	if err != nil {
		t.Log(err)
		t.Fatal("Error should not be raised")
	}

	secret1AnnotationValueExpected := vaultSecretsPath + "/k8s-ns1/" + tvsd.secretsList[0]
	if secret1.Annotations[annotationName] != secret1AnnotationValueExpected {
		t.Fatalf("Incorrect value for secret annotation '%s': '%s'. Expected '%s'", annotationName, secret1.Annotations[annotationName], secret1AnnotationValueExpected)
	}

	secret1TestKey := "testKey-" + tvsd.secretsList[0]
	secret1TestKeyValueExpected := "testValue-" + tvsd.secretsList[0]
	secret1TestKeyValue := string([]byte(secret1.Data[secret1TestKey]))
	if secret1TestKeyValue != secret1TestKeyValueExpected {
		t.Fatalf("Incorrect value for secret key '%s': '%s'. Expected '%s'", secret1TestKey, secret1TestKeyValue, secret1TestKeyValueExpected)
	}

	// Secret2 should be ignored as it was "manually" created in k8s with wrong annotation
	secret2, err := d.testK8sServerReadTestSecret(t, tvsd.secretsList[1]+"-v1", "k8s-ns1")
	if err != nil {
		t.Log(err)
		t.Fatal("Error should not be raised")
	}

	if len(secret2.Annotations[annotationName]) != 0 {
		t.Fatalf("Annotation '%s' created, but it should not", annotationName)
	}

	secret2TestKey := "testK8sKey-" + tvsd.secretsList[1] + "-v1"
	secret2TestKeyValueExpected := "testK8sValue-" + tvsd.secretsList[1] + "-v1"
	secret2TestKeyValue := string([]byte(secret2.Data[secret2TestKey]))
	if secret2TestKeyValue != secret2TestKeyValueExpected {
		t.Fatalf("Incorrect value for secret key '%s': '%s'. Expected '%s'", secret2TestKey, secret2TestKeyValue, secret2TestKeyValueExpected)
	}

	// Secret3 should be ignored as it was "manually" created in k8s with correct annotation name but wrong value
	secret3, err := d.testK8sServerReadTestSecret(t, tvsd.secretsList[2]+"-v1", "k8s-ns1")
	if err != nil {
		t.Log(err)
		t.Fatal("Error should not be raised")
	}

	secret3AnnotationValueExpected := "annotation-value"
	if secret3.Annotations[annotationName] != secret3AnnotationValueExpected {
		t.Fatalf("Incorrect value for secret annotation '%s': '%s'. Expected '%s'", annotationName, secret3.Annotations[annotationName], secret3AnnotationValueExpected)
	}

	secret3TestKey := "testK8sKey-" + tvsd.secretsList[2] + "-v1"
	secret3TestKeyValueExpected := "testK8sValue-" + tvsd.secretsList[2] + "-v1"
	secret3TestKeyValue := string([]byte(secret3.Data[secret3TestKey]))
	if secret3TestKeyValue != secret3TestKeyValueExpected {
		t.Fatalf("Incorrect value for secret key '%s': '%s'. Expected '%s'", secret3TestKey, secret3TestKeyValue, secret3TestKeyValueExpected)
	}

	// Secret4 should be skipped as this version already in k8s
	secret4, err := d.testK8sServerReadTestSecret(t, tvsd.secretsList[3]+"-v1", "k8s-ns1")
	if err != nil {
		t.Log(err)
		t.Fatal("Error should not be raised")
	}

	secret4AnnotationValueExpected := vaultSecretsPath + "/k8s-ns1/" + tvsd.secretsList[3]
	if secret4.Annotations[annotationName] != secret4AnnotationValueExpected {
		t.Fatalf("Incorrect value for secret annotation '%s': '%s'. Expected '%s'", annotationName, secret4.Annotations[annotationName], secret4AnnotationValueExpected)
	}

	secret4TestKey := "testK8sKey-" + tvsd.secretsList[3] + "-v1"
	secret4TestKeyValueExpected := "testK8sValue-" + tvsd.secretsList[3] + "-v1"
	secret4TestKeyValue := string([]byte(secret4.Data[secret4TestKey]))
	if secret4TestKeyValue != secret4TestKeyValueExpected {
		t.Fatalf("Incorrect value for secret key '%s': '%s'. Expected '%s'", secret4TestKey, secret4TestKeyValue, secret4TestKeyValueExpected)
	}

	// Secret5 should be ignored as data for it was deleted from Vault
	secret5, err := d.testK8sServerReadTestSecret(t, tvsd.secretsList[4]+"-v1", "k8s-ns1")
	if err != nil {
		t.Log(err)
		t.Fatal("Error should not be raised")
	}

	secret5AnnotationValueExpected := vaultSecretsPath + "/k8s-ns1/" + tvsd.secretsList[4]
	if secret5.Annotations[annotationName] != secret5AnnotationValueExpected {
		t.Fatalf("Incorrect value for secret annotation '%s': '%s'. Expected '%s'", annotationName, secret5.Annotations[annotationName], secret5AnnotationValueExpected)
	}

	secret5TestKey := "testK8sKey-" + tvsd.secretsList[4] + "-v1"
	secret5TestKeyValueExpected := "testK8sValue-" + tvsd.secretsList[4] + "-v1"
	secret5TestKeyValue := string([]byte(secret5.Data[secret5TestKey]))
	if secret5TestKeyValue != secret5TestKeyValueExpected {
		t.Fatalf("Incorrect value for secret key '%s': '%s'. Expected '%s'", secret5TestKey, secret5TestKeyValue, secret5TestKeyValueExpected)
	}

	// Secret6 should be created
	secret6, err := d.testK8sServerReadTestSecret(t, tvsd.secretsList[5]+"-v1", "k8s-ns1")
	if err != nil {
		t.Log(err)
		t.Fatal("Error should not be raised")
	}

	secret6AnnotationValueExpected := vaultSecretsPath + "/k8s-ns1/" + tvsd.secretsList[5]
	if secret6.Annotations[annotationName] != secret6AnnotationValueExpected {
		t.Fatalf("Incorrect value for secret annotation '%s': '%s'. Expected '%s'", annotationName, secret6.Annotations[annotationName], secret6AnnotationValueExpected)
	}

	secret6TestKey := "testKey-" + tvsd.secretsList[5]
	secret6TestKeyValueExpected := "testValue-" + tvsd.secretsList[5]
	secret6TestKeyValue := string([]byte(secret6.Data[secret6TestKey]))
	if secret6TestKeyValue != secret6TestKeyValueExpected {
		t.Fatalf("Incorrect value for secret key '%s': '%s'. Expected '%s'", secret6TestKey, secret6TestKeyValue, secret6TestKeyValueExpected)
	}
}

// Test create secrets in k8s with bad name
func TestUpdateSecretsInK8sWithBadSecretName(t *testing.T) {
	var err error
	d := &vtkData{}
	tvsd := d.testVaultServer(t)
	defer tvsd.server.Close()
	_ = d.testK8sServer(t)

	// Create secrets with wrong name
	secretsList := []string{
		"secret-Bad1",
		"secret_bad2",
	}
	d.testVaultServerCreateSecrets(t, secretsList, "k8s-ns1")

	// Get secrets for versioning NS
	filteredSecrets := d.filterSecrets(secretsList, "."+k8sClusterName, "k8s-ns-ver")

	// Get list of k8s secrets
	k8sSecrets, err := d.k8sSecretsList("k8s-ns1")
	if err != nil {
		t.Log(err)
		t.Fatal("Error should not be raised")
	}

	// Test create/update secrets in k8s
	numWorkers = 3
	usjc := make(chan secretForUpdate, numWorkers)
	usrc := make(chan updateSecretResults, numWorkers)

	// WaitGroup is used to wait for the program to finish goroutines
	var wg sync.WaitGroup
	// Use context for cancelation signal in goroutines if errors occurs
	_, cancel := context.WithCancel(context.Background())
	defer cancel() // Make sure it's called to release resources even if no errors

	// Create goroutines
	wg.Add(numWorkers)
	for w := 1; w <= numWorkers; w++ {
		go d.updateSecretsInK8s(cancel, w, &wg, usjc, usrc, "k8s-ns1", "."+k8sClusterName, k8sSecrets)
	}

	// Send secrets to goroutines
	go func() {
		for filteredSecret, versioning := range filteredSecrets {
			usjc <- secretForUpdate{name: filteredSecret, versioning: versioning}
		}
		close(usjc)
	}()

	for i := 1; i <= len(filteredSecrets); i++ {
		usrcResult := <-usrc
		if usrcResult.err != nil {
			wg.Wait()
			t.Log(usrcResult.err)
			t.Fatal("Error should not be raised")
		}
	}
	close(usrc)

	// Secret1 should be ignored as it has wrong name (contain uppercase character)
	_, err = d.testK8sServerReadTestSecret(t, secretsList[0]+"-v1", "k8s-ns1")
	if err == nil {
		t.Fatal("Expected error, but it wasn't returned")
	}

	if !strings.Contains(err.Error(), "secrets \""+secretsList[0]+"-v1\" not found") {
		t.Log(err)
		t.Fatal("Incorrect error response")
	}

	// Secret2 should be ignored as it has wrong name (contain '_' character)
	_, err = d.testK8sServerReadTestSecret(t, secretsList[1]+"-v1", "k8s-ns1")
	if err == nil {
		t.Fatal("Expected error, but it wasn't returned")
	}

	if !strings.Contains(err.Error(), "secrets \""+secretsList[1]+"-v1\" not found") {
		t.Log(err)
		t.Fatal("Incorrect error response")
	}
}

// Test create secrets in k8s with incorrect data
func TestUpdateSecretsInK8sWithIncorrectData(t *testing.T) {
	var err error
	d := &vtkData{}
	tvsd := d.testVaultServer(t)
	defer tvsd.server.Close()
	_ = d.testK8sServer(t)

	// Create secrets with incorrect data
	secretsList := []string{
		"secret-incorrect-data",
	}
	d.testVaultServerCreateSecretsIncorrectData(t, secretsList, "k8s-ns1")

	// Get secrets for versioning NS
	filteredSecrets := d.filterSecrets(secretsList, "."+k8sClusterName, "k8s-ns-ver")

	// Get list of k8s secrets
	k8sSecrets, err := d.k8sSecretsList("k8s-ns1")
	if err != nil {
		t.Log(err)
		t.Fatal("Error should not be raised")
	}

	// Test create/update secrets in k8s
	numWorkers = 3
	usjc := make(chan secretForUpdate, numWorkers)
	usrc := make(chan updateSecretResults, numWorkers)

	// WaitGroup is used to wait for the program to finish goroutines
	var wg sync.WaitGroup
	// Use context for cancelation signal in goroutines if errors occurs
	_, cancel := context.WithCancel(context.Background())
	defer cancel() // Make sure it's called to release resources even if no errors

	// Create goroutines
	wg.Add(numWorkers)
	for w := 1; w <= numWorkers; w++ {
		go d.updateSecretsInK8s(cancel, w, &wg, usjc, usrc, "k8s-ns1", "."+k8sClusterName, k8sSecrets)
	}

	// Send secrets to goroutines
	go func() {
		for filteredSecret, versioning := range filteredSecrets {
			usjc <- secretForUpdate{name: filteredSecret, versioning: versioning}
		}
		close(usjc)
	}()

	for i := 1; i <= len(filteredSecrets); i++ {
		usrcResult := <-usrc
		if usrcResult.err != nil {
			wg.Wait()
			t.Log(usrcResult.err)
			t.Fatal("Error should not be raised")
		}
	}
	close(usrc)

	// Secret should be ignored as it has incorrect data
	_, err = d.testK8sServerReadTestSecret(t, secretsList[0]+"-v1", "k8s-ns1")
	if err == nil {
		t.Fatal("Expected error, but it wasn't returned")
	}

	if !strings.Contains(err.Error(), "secrets \""+secretsList[0]+"-v1\" not found") {
		t.Log(err)
		t.Fatal("Incorrect error response")
	}
}

// Test create/update secrets in k8s without versioning
func TestUpdateSecretsInK8sNonVersioningNS(t *testing.T) {
	d := &vtkData{}
	tvsd := d.testVaultServer(t)
	defer tvsd.server.Close()
	_ = d.testK8sServer(t)

	// Create secret2 in k8s with correct annotation
	k8sSecret2Name := strings.TrimSuffix(tvsd.secretsList[1], "."+k8sClusterName)
	d.testK8sServerCreateSecret(t, k8sSecret2Name, "k8s-ns1", annotationName, vaultSecretsPath+"/k8s-ns1/"+tvsd.secretsList[1])

	// Get secrets for non-versioning NS
	filteredSecrets := d.filterSecrets(tvsd.secretsList, "."+k8sClusterName, "k8s-ns-nonver")

	// Get list of k8s secrets
	k8sSecrets, err := d.k8sSecretsList("k8s-ns1")
	if err != nil {
		t.Log(err)
		t.Fatal("Error should not be raised")
	}

	// Test create/update secrets in k8s
	numWorkers = 5
	usjc := make(chan secretForUpdate, numWorkers)
	usrc := make(chan updateSecretResults, numWorkers)

	// WaitGroup is used to wait for the program to finish goroutines
	var wg sync.WaitGroup
	// Use context for cancelation signal in goroutines if errors occurs
	_, cancel := context.WithCancel(context.Background())
	defer cancel() // Make sure it's called to release resources even if no errors

	// Create goroutines
	wg.Add(numWorkers)
	for w := 1; w <= numWorkers; w++ {
		go d.updateSecretsInK8s(cancel, w, &wg, usjc, usrc, "k8s-ns1", "."+k8sClusterName, k8sSecrets)
	}

	// Send secrets to goroutines
	go func() {
		for filteredSecret, versioning := range filteredSecrets {
			usjc <- secretForUpdate{name: filteredSecret, versioning: versioning}
		}
		close(usjc)
	}()

	for i := 1; i <= len(filteredSecrets); i++ {
		usrcResult := <-usrc
		if usrcResult.err != nil {
			wg.Wait()
			t.Log(usrcResult.err)
			t.Fatal("Error should not be raised")
		}
	}
	close(usrc)

	// Secret1 should be created in k8s with versioning
	secret1, err := d.testK8sServerReadTestSecret(t, tvsd.secretsList[0]+"-v2", "k8s-ns1")
	if err != nil {
		t.Log(err)
		t.Fatal("Error should not be raised")
	}

	secret1AnnotationValueExpected := vaultSecretsPath + "/k8s-ns1/" + tvsd.secretsList[0]
	if secret1.Annotations[annotationName] != secret1AnnotationValueExpected {
		t.Fatalf("Incorrect value for secret annotation '%s': '%s'. Expected '%s'", annotationName, secret1.Annotations[annotationName], secret1AnnotationValueExpected)
	}

	secret1TestKey := "testKey-" + tvsd.secretsList[0]
	secret1TestKeyValueExpected := "testValue-" + tvsd.secretsList[0]
	secret1TestKeyValue := string([]byte(secret1.Data[secret1TestKey]))
	if secret1TestKeyValue != secret1TestKeyValueExpected {
		t.Fatalf("Incorrect value for secret key '%s': '%s'. Expected '%s'", secret1TestKey, secret1TestKeyValue, secret1TestKeyValueExpected)
	}

	// Secret2 should be created in k8s with versioning
	secret2, err := d.testK8sServerReadTestSecret(t, tvsd.secretsList[1]+"-v1", "k8s-ns1")
	if err != nil {
		t.Log(err)
		t.Fatal("Error should not be raised")
	}

	secret2AnnotationValueExpected := vaultSecretsPath + "/k8s-ns1/" + tvsd.secretsList[1]
	if secret2.Annotations[annotationName] != secret2AnnotationValueExpected {
		t.Fatalf("Incorrect value for secret annotation '%s': '%s'. Expected '%s'", annotationName, secret2.Annotations[annotationName], secret2AnnotationValueExpected)
	}

	secret2TestKey := "testKey-" + tvsd.secretsList[1]
	secret2TestKeyValueExpected := "testValue-" + tvsd.secretsList[1]
	secret2TestKeyValue := string([]byte(secret2.Data[secret2TestKey]))
	if secret2TestKeyValue != secret2TestKeyValueExpected {
		t.Fatalf("Incorrect value for secret key '%s': '%s'. Expected '%s'", secret2TestKey, secret2TestKeyValue, secret2TestKeyValueExpected)
	}

	// Secret2 should be updated in k8s without versioning and without suffix "cluster name"
	secret2NonV, err := d.testK8sServerReadTestSecret(t, k8sSecret2Name, "k8s-ns1")
	if err != nil {
		t.Log(err)
		t.Fatal("Error should not be raised")
	}

	secret2AnnotationValueExpectedNonV := vaultSecretsPath + "/k8s-ns1/" + tvsd.secretsList[1]
	if secret2NonV.Annotations[annotationName] != secret2AnnotationValueExpectedNonV {
		t.Fatalf("Incorrect value for secret annotation '%s': '%s'. Expected '%s'", annotationName, secret2NonV.Annotations[annotationName], secret2AnnotationValueExpectedNonV)
	}

	secret2TestKeyNonV := "testKey-" + tvsd.secretsList[1]
	secret2TestKeyValueExpectedNonV := "testValue-" + tvsd.secretsList[1]
	secret2TestKeyValueNonV := string([]byte(secret2NonV.Data[secret2TestKeyNonV]))
	if secret2TestKeyValueNonV != secret2TestKeyValueExpectedNonV {
		t.Fatalf("Incorrect value for secret key '%s': '%s'. Expected '%s'", secret2TestKeyNonV, secret2TestKeyValueNonV, secret2TestKeyValueExpectedNonV)
	}
}
