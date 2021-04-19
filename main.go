/*
* Author (C) 2019 Oleh Halytskyi
*
 */

package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	vault "github.com/hashicorp/vault/api"
	"github.com/pkg/errors"

	k8sCoreV1 "k8s.io/api/core/v1"
	k8sApiErr "k8s.io/apimachinery/pkg/api/errors"
	k8sMetaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	debug                           string
	appName                         string
	podNamespace                    string
	vaultAddr                       string
	vaultNamespace                  string
	authMethod                      string
	vaultToken                      string
	approleRoleID                   string
	approleSecretIDWrappedToken     string
	approleSecretIDWrappedTokenFile string
	tokenRotationInterval           int
	approleSecretIDRotationInterval int
	numWorkers                      int
	syncInterval                    int
	k8sClusterName                  string
	vaultSecretsPath                string
	nonVersioningNamespaces         string
	annotationName                  string
	prometheusMetrics               string
	prometheusListenAddress         string
	prometheusMetricsPath           string
)

// VTK Data
type vtkData struct {
	vaultClient                 *vault.Client        // Vault client
	k8sClient                   kubernetes.Interface // K8s client
	vaultTokenAccessor          string               // Vault Token Accessor
	vaultTokenTTL               map[string]int64     // Vault Token TTL
	approleSecretID             interface{}          // Vault AppRole Secret ID
	approleName                 string               // Vault AppRole Name
	approleSecretIDTTL          map[string]int64     // Vault AppRole Secret ID TTL
	nonVersioningNamespacesList []string             // List of non-versioning namespaces
}

// Secret for update in k8s
type secretForUpdate struct {
	name       string
	versioning int
}

// K8s update secret results
type updateSecretResults struct {
	created float64
	updated float64
	skipped float64
	synced  float64
	err     error
}

// Get 'string' environment variable or return default value
func getEnvWithDefaultString(envName, defValue string) string {
	envValue := os.Getenv(envName)
	if envValue != "" {
		return envValue
	}

	return defValue
}

// Get 'int' environment variable or return default value
func getEnvWithDefaultInt(envName string, defValue int) int {
	envValue := os.Getenv(envName)
	if envValue != "" {
		envValueInt, err := strconv.Atoi(envValue)
		if err != nil {
			glog.Fatal(err)
		}
		return envValueInt
	}

	return defValue
}

// Get pod namespace variable
func getEnvPodNamespace() string {
	envVal := os.Getenv("POD_NAMESPACE")
	if envVal == "" {
		if data, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
			if ns := strings.TrimSpace(string(data)); len(ns) > 0 {
				envVal = ns
			} else {
				glog.Fatal(err)
			}
		} else {
			glog.Fatal(err)
		}
	}

	return envVal
}

// Verify config parameters
func verifyConfig() error {
	if vaultAddr == "" {
		return fmt.Errorf("Must set variable VAULT_ADDR")
	}

	if vaultNamespace == "" {
		return fmt.Errorf("Must set variable VAULT_NAMESPACE")
	}
	os.Setenv("VAULT_NAMESPACE", vaultNamespace)

	if authMethod == "" {
		return fmt.Errorf("You must provide an auth method. Parameter AUTH_METHOD can be \"token\" or \"approle\"")
	}
	if authMethod == "token" {
		if vaultToken == "" {
			return fmt.Errorf("VAULT_TOKEN should be defined for \"token\" auth method")
		}
	} else if authMethod == "approle" {
		if approleRoleID == "" {
			return fmt.Errorf("APPROLE_ROLE_ID should be defined for \"approle\" auth method")
		}
		if approleSecretIDWrappedTokenFile == "" {
			if approleSecretIDWrappedToken == "" {
				return fmt.Errorf("APPROLE_SECRET_ID_WRAPPED_TOKEN should be defined for \"approle\" auth method")
			}
		} else {
			data, err := ioutil.ReadFile(approleSecretIDWrappedTokenFile)
			if err != nil {
				return errors.Wrap(err, "Failed to get AppRole Secret ID Wrapped Token from file '"+approleSecretIDWrappedTokenFile+"'")
			}
			approleSecretIDWrappedToken = string(data)
		}
	} else {
		return fmt.Errorf("Incorrect value for AUTH_METHOD, can be \"token\" or \"approle\"")
	}

	if k8sClusterName == "" {
		return fmt.Errorf("Must set variable K8S_CLUSTER_NAME")
	}

	vaultSecretsPath = strings.TrimSuffix(vaultSecretsPath, "/")
	if vaultSecretsPath == "" {
		return fmt.Errorf("Must set variable SECRETS_PATH_VAULT")
	}

	return nil
}

// Define struct parameters
func structParams() (*vtkData, error) {
	var err error
	d := &vtkData{}

	// Make Vault client
	d.vaultClient, err = newVaultClient(vaultAddr)
	if err != nil {
		return nil, err
	}

	// Make k8s client
	d.k8sClient, err = newK8sClient()
	if err != nil {
		return nil, err
	}

	if nonVersioningNamespaces != "" {
		d.nonVersioningNamespacesList = strings.Split(nonVersioningNamespaces, ",")
	}

	return d, nil
}

// Create a new Vault client
func newVaultClient(vaultAddr string) (*vault.Client, error) {
	vconfig := vault.DefaultConfig()
	vconfig.Address = vaultAddr
	vclient, err := vault.NewClient(vconfig)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to get Vault config")
	}

	return vclient, nil
}

// Create a new k8s client
func newK8sClient() (*kubernetes.Clientset, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, errors.Wrap(err, "Failed to get k8s config")
	}
	k8sClientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to get k8s 'k8sClientSet'")
	}

	return k8sClientSet, nil
}

// Verify if mount exists in Vault and has correct engine type and version
func (d *vtkData) verifyVaultMount() error {
	mountNotExists := true
	vaultMount := strings.SplitN(vaultSecretsPath, "/", 2)[0] + "/"
	vaultMountsIn, err := d.vaultClient.Sys().ListMounts()
	if err != nil {
		return errors.Wrap(err, "Failed to get list Vault mounts")
	}
	for k, m := range vaultMountsIn {
		if !strings.HasPrefix(vaultMount, k) {
			continue
		}

		if m.Type != "kv" {
			return fmt.Errorf("Matching mount '%s' for path '%s' is not of type kv", k, vaultMount)
		}

		kvVersion, _ := strconv.Atoi(m.Options["version"])
		if kvVersion != 2 {
			return fmt.Errorf("Vault mount '%s' and defined path '%s' matched but Vault mount version is not '2'", k, vaultMount)
		}
		mountNotExists = false
	}
	if mountNotExists {
		return fmt.Errorf("Mount path: '%s' doesn't exists in Vault", vaultMount)
	}

	return nil
}

// Sync Vault secrets to k8s
func (d *vtkData) syncVaultToK8s() {
	k8sClusterNameSuffix := "." + k8sClusterName

	for range time.Tick(time.Second * time.Duration(syncInterval)) {
		startSync := time.Now()
		glog.V(2).Infoln()
		glog.V(2).Infoln("Started sync secrets from Vault to k8s")

		// Get list of Vault namespaces
		vaultNamespaces, err := d.vaultNamespacesList()
		if err != nil {
			glog.Errorln(err)
			syncTime.Set(float64(0))
			syncStatus.WithLabelValues("-").Set(0)
			continue
		}
		if len(vaultNamespaces) == 0 {
			glog.Warningln("Didn't find any namespaces under secret path:", vaultSecretsPath)
			syncTime.Set(float64(0))
			syncStatus.WithLabelValues("-").Set(0)
			continue
		}
		glog.V(2).Infoln("Namespaces in Vault:", vaultNamespaces)

		// Get list of K8s namespaces
		k8sNamespaces, err := d.k8sNamespacesList()
		if err != nil {
			glog.Errorln(err)
			syncTime.Set(float64(0))
			syncStatus.WithLabelValues("-").Set(0)
			continue
		}
		glog.V(2).Infoln("Namespaces in K8s:", k8sNamespaces)

		// Get list of namespaces which should be synced
		nsForSync := d.namespacesForSync(vaultNamespaces, k8sNamespaces)
		if len(nsForSync) == 0 {
			glog.Warningln("There is no namespaces in Vault which exists on current cluster for sync")
			syncTime.Set(float64(0))
			syncStatus.WithLabelValues("-").Set(0)
			continue
		}
		glog.V(2).Infoln("Namespaces for sync:", nsForSync)

		// Sync secrets for each namespace
		for _, namespace := range nsForSync {
			syncStatusNamespace := 1.0

			// Get list of Vault secrets
			secrets, err := d.secretsList(namespace)
			if err != nil {
				glog.Errorln(err)
				syncStatus.WithLabelValues(namespace).Set(0)
				continue
			}
			glog.V(2).Infoln()
			glog.V(2).Infoln("Secrets in Vault under '"+namespace+"' namespace:", secrets)

			// Filter secrets
			filteredSecrets := d.filterSecrets(secrets, k8sClusterNameSuffix, namespace)
			glog.V(2).Infoln("Filtered secrets in Vault under '"+namespace+"' namespace:", filteredSecrets)

			// Get list of k8s secrets
			k8sSecrets, err := d.k8sSecretsList(namespace)
			if err != nil {
				glog.Errorln(err)
				syncStatus.WithLabelValues(namespace).Set(0)
				continue
			}
			glog.V(2).Infoln("Secrets in k8s '"+namespace+"' namespace:", k8sSecrets)

			// Create/update secrets in k8s
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
				go d.updateSecretsInK8s(cancel, w, &wg, usjc, usrc, namespace, k8sClusterNameSuffix, k8sSecrets)
			}

			// Send secrets to goroutines
			go func() {
				for filteredSecret, versioning := range filteredSecrets {
					usjc <- secretForUpdate{name: filteredSecret, versioning: versioning}
				}
				close(usjc)
			}()

			// Receive results from goroutines
			updateResults := &updateSecretResults{
				created: 0,
				updated: 0,
				skipped: 0,
				synced:  0,
				err:     nil,
			}
			for i := 1; i <= len(filteredSecrets); i++ {
				usrcResult := <-usrc
				if usrcResult.err != nil {
					wg.Wait()
					glog.Errorln(usrcResult.err)
					syncStatusNamespace = 0
					break
				}
				updateResults.created += usrcResult.created
				updateResults.updated += usrcResult.updated
				updateResults.skipped += usrcResult.skipped
				updateResults.synced += usrcResult.synced
			}
			close(usrc)

			glog.V(2).Infoln("Created secrets:", updateResults.created)
			glog.V(2).Infoln("Updated secrets:", updateResults.updated)
			glog.V(2).Infoln("Skipped secrets:", updateResults.skipped)
			glog.V(2).Infoln("Synced secrets:", updateResults.synced)
			secretsCreated.WithLabelValues(namespace).Set(updateResults.created)
			secretsUpdated.WithLabelValues(namespace).Set(updateResults.updated)
			secretsSkipped.WithLabelValues(namespace).Set(updateResults.skipped)
			secretsSynced.WithLabelValues(namespace).Set(updateResults.synced)
			syncStatus.WithLabelValues(namespace).Set(syncStatusNamespace)
		}

		glog.V(2).Infoln("Finished sync")
		endSync := time.Since(startSync)
		glog.V(2).Infoln("Sync time:", endSync)
		syncTime.Set(float64(endSync))
		syncStatus.WithLabelValues("-").Set(1)
		syncCount.Inc()
		glog.V(2).Infoln("Total number of syncs:", readMetricValue(syncCount))
	}
}

// List namespaces from Vault
func (d *vtkData) vaultNamespacesList() ([]string, error) {
	vaultMount := strings.SplitN(vaultSecretsPath, "/", 2)[0]
	vaultNamespacesMount := strings.SplitN(vaultSecretsPath, "/", 2)[1]
	mountPath := vaultMount + "/metadata/" + vaultNamespacesMount

	// Get mount list from Vault
	ml, err := d.vaultClient.Logical().List(mountPath)
	if err != nil {
		return nil, err
	}
	if ml == nil || ml.Data == nil {
		return nil, nil
	}

	// Get namespaces list from Vault
	vaultNamespaces := []string{}
	for _, v := range ml.Data["keys"].([]interface{}) {
		if strings.HasSuffix(v.(string), "/") {
			vaultNamespaces = append(vaultNamespaces, strings.TrimSuffix(v.(string), "/"))
		}
	}

	return vaultNamespaces, nil
}

// List of K8s namespaces
func (d *vtkData) k8sNamespacesList() ([]string, error) {
	k8sNSObj, err := d.k8sClient.CoreV1().Namespaces().List(k8sMetaV1.ListOptions{})
	if err != nil {
		return nil, err
	}
	k8sNamespaces := []string{}
	for _, v := range k8sNSObj.Items {
		k8sNamespaces = append(k8sNamespaces, v.Name)
	}

	return k8sNamespaces, nil
}

// List of namespaces which should be synced
func (d *vtkData) namespacesForSync(vaultNamespaces []string, k8sNamespaces []string) []string {
	nsForSync := []string{}
	k8sNamespacesMap := make(map[string]bool)

	for i := range k8sNamespaces {
		k8sNamespacesMap[k8sNamespaces[i]] = true
	}

	for y := range vaultNamespaces {
		if k8sNamespacesMap[vaultNamespaces[y]] {
			nsForSync = append(nsForSync, vaultNamespaces[y])
		} else {
			syncStatus.WithLabelValues(vaultNamespaces[y]).Set(0)
		}
	}

	return nsForSync
}

// List secrets from Vault
func (d *vtkData) secretsList(namespace string) ([]string, error) {
	vaultMount := strings.SplitN(vaultSecretsPath, "/", 2)[0]
	vaultSecretsMount := strings.SplitN(vaultSecretsPath, "/", 2)[1]
	mountPath := vaultMount + "/metadata/" + vaultSecretsMount + "/" + namespace

	// Get mount list from Vault
	ml, err := d.vaultClient.Logical().List(mountPath)
	if err != nil {
		return nil, err
	}
	if ml == nil || ml.Data == nil {
		return nil, nil
	}

	// Get secrets list from Vault
	vaultSecrets := []string{}
	for _, v := range ml.Data["keys"].([]interface{}) {
		if !strings.HasSuffix(v.(string), "/") {
			vaultSecrets = append(vaultSecrets, v.(string))
		}
	}

	return vaultSecrets, nil
}

// Read secrets from Vault
func (d *vtkData) secretsRead(vaultSecretPath string) (map[string]interface{}, string, error) {
	vaultMount := strings.SplitN(vaultSecretPath, "/", 2)[0]
	vaultSecretsMount := strings.SplitN(vaultSecretPath, "/", 2)[1]
	mountPath := vaultMount + "/data/" + vaultSecretsMount

	s, err := d.vaultClient.Logical().Read(mountPath)
	if err != nil {
		return nil, "", err
	}
	if s == nil || s.Data == nil || s.Data["data"] == nil {
		return nil, "", nil
	}

	secretVersion := fmt.Sprintf("%s", s.Data["metadata"].(map[string]interface{})["version"])

	return s.Data["data"].(map[string]interface{}), secretVersion, nil
}

func (d *vtkData) filterSecrets(secrets []string, k8sClusterNameSuffix, namespace string) map[string]int {
	filteredSecrets := make(map[string]int)

	// Check if non-versioning namespace
	nonVersioningNamespace := false
	if len(d.nonVersioningNamespacesList) != 0 {
		for _, nvn := range d.nonVersioningNamespacesList {
			if nvn == namespace {
				nonVersioningNamespace = true
				break
			}
		}
	}

	for _, secret := range secrets {
		if nonVersioningNamespace == true {
			if strings.HasSuffix(secret, k8sClusterNameSuffix) {
				filteredSecrets[secret] = 0
			}
		}
		if _, ok := filteredSecrets[secret]; !ok {
			if strings.Contains(secret, ".") {
				if !strings.HasSuffix(secret, k8sClusterNameSuffix) || strings.Count(secret, ".") > 1 {
					continue
				}
			}
			filteredSecrets[secret] = 1
		}
	}

	return filteredSecrets
}

// List of K8s secrets
func (d *vtkData) k8sSecretsList(namespace string) ([]string, error) {
	k8sNSObj, err := d.k8sClient.CoreV1().Secrets(namespace).List(k8sMetaV1.ListOptions{})
	if err != nil {
		return nil, err
	}
	k8sSecrets := []string{}
	for _, v := range k8sNSObj.Items {
		k8sSecrets = append(k8sSecrets, v.Name)
	}

	return k8sSecrets, nil
}

// Create/update secrets in k8s
func (d *vtkData) updateSecretsInK8s(cancel context.CancelFunc, numWorker int, wg *sync.WaitGroup, usjc chan secretForUpdate, usrc chan updateSecretResults, namespace, k8sClusterNameSuffix string, k8sSecrets []string) {
	// Schedule the call to WaitGroup's Done to tell goroutine is completed
	defer wg.Done()

	// For log messages
	var numWorkerStr string
	if numWorkers > 1 {
		numWorkerStr = "[Worker #" + strconv.Itoa(numWorker) + "]: "
	}

START_LOOP:
	for secretForUpdate := range usjc {
		updateResults := &updateSecretResults{
			created: 0,
			updated: 0,
			skipped: 0,
			synced:  0,
			err:     nil,
		}

		// Read secrets
		vaultSecretPathFull := vaultSecretsPath + "/" + namespace + "/" + secretForUpdate.name
		glog.V(2).Infoln(numWorkerStr + "Read '" + vaultSecretPathFull + "' from Vault")
		s, v, err := d.secretsRead(vaultSecretPathFull)
		if err != nil {
			updateResults.err = errors.Wrap(err, "Error during read Vault secret")
			usrc <- *updateResults
			cancel()
			return
		}
		if len(s) == 0 {
			glog.V(2).Infoln(numWorkerStr+"Didn't get any data for secret:", vaultSecretPathFull, ", skipped")
			updateResults.skipped++
			usrc <- *updateResults
			continue
		}
		// Convert data (should be base64 encoded in k8s)
		data := make(map[string][]byte)
		for k, v := range s {
			if reflect.ValueOf(v).Kind() != reflect.String {
				glog.V(2).Infoln(numWorkerStr+"Incorrect data in secret:", vaultSecretPathFull, ", skipped")
				updateResults.skipped++
				usrc <- *updateResults
				continue START_LOOP
			}
			data[k] = []byte(v.(string))
		}

		// Make k8s secret name
		k8sSecretsForUpdate := make(map[string]int)
		k8sSecretsForUpdate[secretForUpdate.name+"-v"+v] = 1
		if secretForUpdate.versioning == 0 {
			k8sSecretsForUpdate[strings.TrimSuffix(secretForUpdate.name, k8sClusterNameSuffix)] = 0
		}
		glog.V(2).Infoln(numWorkerStr+"Secrets that need to check before create/update:", k8sSecretsForUpdate)

		// Verify if we should update versioning secrets in k8s
		for k8sSecret, k8sSecretVersioning := range k8sSecretsForUpdate {
			if k8sSecretVersioning == 1 {
				for y := range k8sSecrets {
					if k8sSecret == k8sSecrets[y] {
						delete(k8sSecretsForUpdate, k8sSecret)
						glog.V(2).Infoln(numWorkerStr + "Ignoring secret '" + k8sSecret + "' as it already exists in '" + namespace + "' namespace")
						updateResults.synced++
						break
					}
				}
			}
		}
		glog.V(2).Infoln(numWorkerStr+"Secrets that can be created/updated:", k8sSecretsForUpdate)
		if len(k8sSecretsForUpdate) == 0 {
			usrc <- *updateResults
			continue
		}

		// Create/update secrets in k8s
		annotations := make(map[string]string)
		for k8sSecretName := range k8sSecretsForUpdate {
			annotations[annotationName] = vaultSecretPathFull
			secret := &k8sCoreV1.Secret{}
			secret.Name = k8sSecretName
			secret.Data = data
			secret.Annotations = annotations

			// Read k8s secret
			existing, err := d.k8sClient.CoreV1().Secrets(namespace).Get(secret.Name, k8sMetaV1.GetOptions{})

			// Create new secret
			if k8sApiErr.IsNotFound(err) {
				glog.V(2).Infoln(numWorkerStr + "Create k8s secret '" + secret.Name + "' from vault secret '" + vaultSecretPathFull + "'")
				if _, err := d.k8sClient.CoreV1().Secrets(namespace).Create(secret); err != nil {
					glog.Errorln(errors.Wrap(err, numWorkerStr+"Error during create k8s secret"))
					updateResults.skipped++
					continue
				}
				updateResults.created++
				updateResults.synced++
				continue
			} else if err != nil {
				glog.Errorln(numWorkerStr+"Error during get k8s secret:", err)
				updateResults.skipped++
				continue
			}

			// Skip update non-versioning secrets if it already up-to-date in k8s
			if reflect.DeepEqual(existing.Data, secret.Data) == true {
				glog.V(2).Infoln(numWorkerStr + "Ignoring update secret '" + secret.Name + "' in '" + namespace + "' namespace as it already up-to-date")
				updateResults.synced++
				continue
			}

			// Verify annotation
			if _, ok := existing.Annotations[annotationName]; !ok {
				glog.V(2).Infoln(numWorkerStr + "WARNING: Ignoring k8s secret '" + secret.Name + "' in '" + namespace + "' namespace as it not managed by '" + appName + "' application")
				updateResults.skipped++
				continue
			}
			if existing.Annotations[annotationName] != vaultSecretPathFull {
				glog.V(2).Infoln(numWorkerStr+"WARNING: Ignoring k8s secret '"+secret.Name+"' in '"+namespace+"' namespace as annotation for it has different path:", existing.Annotations[annotationName])
				updateResults.skipped++
				continue
			}

			// Update secret
			glog.V(2).Infoln(numWorkerStr + "Update k8s secret '" + secret.Name + "' from vault secret '" + vaultSecretPathFull + "'")
			_, _ = d.k8sClient.CoreV1().Secrets(namespace).Update(secret)
			if _, err = d.k8sClient.CoreV1().Secrets(namespace).Update(secret); err != nil {
				updateResults.err = errors.Wrap(err, "Error during update k8s secret")
				usrc <- *updateResults
				cancel()
				return
			}
			updateResults.updated++
			updateResults.synced++
		}
		usrc <- *updateResults
	}
}

func main() {
	// Configure gLog
	flag.CommandLine.Parse([]string{})
	flag.Set("logtostderr", "true")

	// Config params
	flag.StringVar(&debug, "debug", getEnvWithDefaultString("DEBUG", "false"), "Debug mode")
	flag.StringVar(&appName, "app_name", getEnvWithDefaultString("APP_NAME", "vault-to-k8s"), "Application name")
	flag.StringVar(&podNamespace, "pod_namespace", getEnvPodNamespace(), "Pod namespace in which app runs")
	flag.StringVar(&vaultAddr, "vault_addr", getEnvWithDefaultString("VAULT_ADDR", ""), "URL to Vault server")
	flag.StringVar(&vaultNamespace, "vault_namespace", getEnvWithDefaultString("VAULT_NAMESPACE", ""), "Vault namespace")
	flag.StringVar(&authMethod, "auth_method", getEnvWithDefaultString("AUTH_METHOD", ""), "Auth method in Vault")
	flag.StringVar(&vaultToken, "vault_token", getEnvWithDefaultString("VAULT_TOKEN", ""), "'Token' auth method")
	flag.StringVar(&approleRoleID, "approle_role_id", getEnvWithDefaultString("APPROLE_ROLE_ID", ""), "Vault AppRole Role ID")
	flag.StringVar(&approleSecretIDWrappedToken, "approle_secret_id_wrapped_token", getEnvWithDefaultString("APPROLE_SECRET_ID_WRAPPED_TOKEN", ""), "Vault AppRole wrapped token for getting Secret ID")
	flag.StringVar(&approleSecretIDWrappedTokenFile, "approle_secret_id_wrapped_token_file", getEnvWithDefaultString("APPROLE_SECRET_ID_WRAPPED_TOKEN_FILE", ""), "File with Vault AppRole wrapped token")
	flag.IntVar(&tokenRotationInterval, "token_rotation_interval", getEnvWithDefaultInt("TOKEN_ROTATION_INTERVAL", -1), "Vault Token rotation interval")
	flag.IntVar(&approleSecretIDRotationInterval, "approle_secretid_rotation_interval", getEnvWithDefaultInt("APPROLE_SECRETID_ROTATION_INTERVAL", -1), "Vault AppRole Secret ID rotation interval")
	flag.IntVar(&numWorkers, "num_workers", getEnvWithDefaultInt("NUM_WORKERS", 1), "Number of workers for read/create/update secrets")
	flag.IntVar(&syncInterval, "sync_interval", getEnvWithDefaultInt("SYNC_INTERVAL", 300), "Interval of sync secrets from Vault to k8s")
	flag.StringVar(&k8sClusterName, "k8s_cluster_name", getEnvWithDefaultString("K8S_CLUSTER_NAME", ""), "The name of the Kubernetes cluster where the application is running")
	flag.StringVar(&vaultSecretsPath, "secrets_path_vault", getEnvWithDefaultString("SECRETS_PATH_VAULT", ""), "Paths to secrets in Vault")
	flag.StringVar(&nonVersioningNamespaces, "non_versioning_namespaces", getEnvWithDefaultString("NON_VERSIONING_NAMESPACES", ""), "Non-versioning namespaces")
	flag.StringVar(&annotationName, "annotation_name", getEnvWithDefaultString("ANNOTATION_NAME", "vault-to-k8s/secret"), "Annotation name for k8s Secret object")
	flag.StringVar(&prometheusMetrics, "prometheus_metrics", getEnvWithDefaultString("PROMETHEUS_METRICS", "true"), "Prometheus metrics")
	flag.StringVar(&prometheusListenAddress, "prometheus_listen_address", getEnvWithDefaultString("PROMETHEUS_LISTEN_ADDRESS", ":9703"), "Address on which expose metrics and web interface")
	flag.StringVar(&prometheusMetricsPath, "prometheus_metrics_path", getEnvWithDefaultString("PROMETHEUS_METRICS_PATH", "/metrics"), "Path under which to expose metrics")
	flag.Parse()

	// Debug mode
	if debug == "true" {
		flag.Set("v", "2")
	}

	// Verify config parameters
	if err := verifyConfig(); err != nil {
		glog.Fatal(err)
	}

	// Define struct parameters
	d, err := structParams()
	if err != nil {
		glog.Fatal(err)
	}

	// Authentication
	if authMethod == "approle" {
		// Authenticate in Vault
		if err := d.approleAuthenticate(); err != nil {
			glog.Fatal(err)
		}
		// AppRole Secret ID rotation
		if approleSecretIDRotationInterval != 0 {
			go d.approleSecretIDRotation()
		}
	}

	// Token rotation
	if tokenRotationInterval != 0 {
		go d.tokenRotation()
	}

	// Verify if mount exists in Vault and has correct engine version
	if err := d.verifyVaultMount(); err != nil {
		glog.Fatal(err)
	}

	// Prometheus metrics
	if prometheusMetrics == "true" {
		go prometheusMetricsFunc()
	}

	glog.Infoln("Started '" + appName + "' with sync interval '" + strconv.Itoa(syncInterval) + "' seconds and '" + strconv.Itoa(numWorkers) + "' worker(s)")

	// Run sync Vault secrets to k8s
	d.syncVaultToK8s()
}
