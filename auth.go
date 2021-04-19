package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	k8sCoreV1 "k8s.io/api/core/v1"
	k8sApiErr "k8s.io/apimachinery/pkg/api/errors"
	k8sMetaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Authenticate in Vault
func (d *vtkData) approleAuthenticate() error {
	authByEnv := true
	glog.Infoln("Authentication by AppRole...")
	if err := d.approleGetToken(); err != nil {
		authByEnv = false
		glog.Warningln(err)
		glog.Infoln("Trying to get token by credentials from '" + appName + "-system' secret '" + podNamespace + "' namespace")
		if err := d.approleReadAppSecret(); err != nil {
			return err
		}
		if err := d.approleGetToken(); err != nil {
			return err
		}
	}
	// Revoke old 'token' before replace it by new one
	if err := d.revokeOldToken(); err != nil {
		glog.Errorln(err)
	}
	if authByEnv {
		// Revoke old 'secret_id' before replace it by new one
		if err := d.approleRevokeOldSecretID(); err != nil {
			glog.Errorln(err)
		}
	}
	// Save 'token_accessor' and 'secret_id' to k8s secret object
	if err := d.updateAppSystemSecret(); err != nil {
		return err
	}

	glog.Infoln("Successfully authenticated")

	return nil
}

// Get token
func (d *vtkData) approleGetToken() error {
	// Get AppRole Secret ID from wrapped token
	if d.approleSecretID == nil {
		glog.Infoln("Getting 'secret_id' from wrapped token")
		secretID, err := d.vaultClient.Logical().Unwrap(approleSecretIDWrappedToken)
		if err != nil {
			if strings.Contains(err.Error(), "wrapping token is not valid or does not exist") {
				return fmt.Errorf("There is no valid 'wrapped token' in 'APPROLE_SECRET_ID_WRAPPED_TOKEN' or 'APPROLE_SECRET_ID_WRAPPED_TOKEN_FILE'")
			}
			return err
		}
		d.approleSecretID = secretID.Data["secret_id"]
	}

	// Params for fetch token
	options := map[string]interface{}{
		"role_id":   approleRoleID,
		"secret_id": d.approleSecretID,
	}
	authPath := fmt.Sprintf("auth/%s/login", authMethod)

	// Fetching token
	glog.V(2).Infoln("Fetching token from Vault")
	vaultTokenValues, err := d.vaultClient.Logical().Write(authPath, options)
	if err != nil {
		return err
	}
	// Get token params
	vaultToken = vaultTokenValues.Auth.ClientToken
	d.vaultTokenAccessor = vaultTokenValues.Auth.Accessor
	d.approleName = vaultTokenValues.Auth.Metadata["role_name"]
	d.vaultTokenTTL = make(map[string]int64)
	d.vaultTokenTTL["creation_time"] = time.Now().Unix()
	d.vaultTokenTTL["ttl"] = int64(vaultTokenValues.Auth.LeaseDuration)
	// Set correct value for token interval
	if tokenRotationInterval == -1 {
		tokenRotationInterval = int(float64(d.vaultTokenTTL["ttl"]) * 0.7)
		glog.Infoln("'TOKEN_ROTATION_INTERVAL' wasn't defined, therefore it calculated and set to '" + strconv.Itoa(tokenRotationInterval) + "' seconds")
	} else if tokenRotationInterval >= int(d.vaultTokenTTL["ttl"]) {
		tokenRotationInterval = int(float64(d.vaultTokenTTL["ttl"]) * 0.7)
		glog.Infoln("'TOKEN_ROTATION_INTERVAL' has incorrect value (>= token ttl), therefore it calculated and set to '" + strconv.Itoa(tokenRotationInterval) + "' seconds")
	}

	// Authenticate
	d.vaultClient.SetToken(vaultToken)

	return nil
}

// Read application system k8s secret
func (d *vtkData) approleReadAppSecret() error {
	secretName := appName + "-system"

	appSecret, err := d.k8sClient.CoreV1().Secrets(podNamespace).Get(secretName, k8sMetaV1.GetOptions{})
	if err != nil {
		return err
	}
	d.approleSecretID = string([]byte(appSecret.Data["approle_secret-id"]))

	return nil
}

// Update application system secret
func (d *vtkData) updateAppSystemSecret() error {
	s := make(map[string]interface{})
	s["token-accessor"] = d.vaultTokenAccessor
	s["approle_secret-id"] = d.approleSecretID
	// Convert data (should be base64 encoded in k8s)
	data := make(map[string][]byte)
	for k, v := range s {
		data[k] = []byte(v.(string))
	}

	// Create/update k8s secret in k8s
	annotations := make(map[string]string)
	annotations["createdBy"] = appName
	secretName := appName + "-system"
	secret := &k8sCoreV1.Secret{}
	secret.Name = secretName
	secret.Data = data
	secret.Annotations = annotations

	// Read k8s application  secret
	existing, err := d.k8sClient.CoreV1().Secrets(podNamespace).Get(secret.Name, k8sMetaV1.GetOptions{})

	// Create new secret
	if k8sApiErr.IsNotFound(err) {
		glog.Infoln("Create application secret '" + secret.Name + "' in '" + podNamespace + "' namespace")
		if _, err := d.k8sClient.CoreV1().Secrets(podNamespace).Create(secret); err != nil {
			return errors.Wrap(err, "Error during create application k8s secret")
		}
		return nil
	} else if err != nil {
		return errors.Wrap(err, "Error during get application k8s secret")
	}

	// Verify annotation
	if _, ok := existing.Annotations["createdBy"]; !ok {
		return fmt.Errorf("Can't update application k8s secret '" + secret.Name + "' in '" + podNamespace + "' namespace as it not created by '" + appName + "' application")
	}
	if existing.Annotations["createdBy"] != appName {
		return fmt.Errorf("Secret '%s' already exists in '%s' namespace but it wasn't created by this application", secretName, podNamespace)
	}

	// Update application secret
	glog.V(2).Infoln("Update application secret '" + secret.Name + "' in '" + podNamespace + "' namespace")
	if _, err = d.k8sClient.CoreV1().Secrets(podNamespace).Update(secret); err != nil {
		return errors.Wrap(err, "Error during update k8s secret")
	}

	return nil
}

// Revoke old AppRole Secret ID
func (d *vtkData) approleRevokeOldSecretID() error {
	secretName := appName + "-system"

	glog.V(2).Infoln("Read 'approle_secret-id' from '" + secretName + "' secret")
	appSecret, err := d.k8sClient.CoreV1().Secrets(podNamespace).Get(secretName, k8sMetaV1.GetOptions{})
	if err != nil {
		return err
	}

	// Params for getting 'secret_id_accessor'
	optionsLookupSecretID := map[string]interface{}{
		"secret_id": string([]byte(appSecret.Data["approle_secret-id"])),
	}
	lookupAuthPath := fmt.Sprintf("auth/%s/role/%s/secret-id/lookup", authMethod, d.approleName)

	// Get 'secret_id_accessor' for 'secret_id'
	lookupSecretID, err := d.vaultClient.Logical().Write(lookupAuthPath, optionsLookupSecretID)
	if err != nil {
		return err
	}
	if lookupSecretID == nil {
		glog.V(2).Infoln("There is no valid 'secret_id' for revoke from '" + secretName + "' secret")
		return nil
	}

	// Params for revoke of 'secret_id'
	optionsDestroySecretID := map[string]interface{}{
		"secret_id_accessor": lookupSecretID.Data["secret_id_accessor"],
	}
	destroyAuthPath := fmt.Sprintf("auth/%s/role/%s/secret-id-accessor/destroy", authMethod, d.approleName)

	// Revoking old 'secret_id' by 'secret_id_accessor' (to revoke both)
	glog.V(2).Infoln("Revoking old AppRole Secret ID from '" + secretName + "' secret")
	_, err = d.vaultClient.Logical().Write(destroyAuthPath, optionsDestroySecretID)
	if err != nil {
		return err
	}

	return nil
}

// Revoke old Token
func (d *vtkData) revokeOldToken() error {
	secretName := appName + "-system"

	glog.V(2).Infoln("Read 'token-accessor' from '" + secretName + "' secret")
	appSecret, err := d.k8sClient.CoreV1().Secrets(podNamespace).Get(secretName, k8sMetaV1.GetOptions{})
	if err != nil {
		return err
	}
	vaultTokenAccessor := string([]byte(appSecret.Data["token-accessor"]))
	if vaultTokenAccessor == "" {
		glog.V(2).Infoln("There is no 'token-accessor' in '" + secretName + "' secret")
		return nil
	}

	// Params for 'token' revoke
	optionsDestroyToken := map[string]interface{}{
		"accessor": vaultTokenAccessor,
	}
	revokeAuthPath := fmt.Sprintf("auth/token/revoke-accessor")

	// Revoking old 'token' by 'token_accessor' (to revoke both)
	glog.V(2).Infoln("Revoking old 'token' by 'token_accessor' from '" + secretName + "' secret")
	_, err = d.vaultClient.Logical().Write(revokeAuthPath, optionsDestroyToken)
	if err != nil {
		if strings.Contains(err.Error(), "invalid accessor") {
			glog.V(2).Infoln("There is no valid 'token-accessor' in '" + secretName + "' secret for revoke 'token'")
		} else {
			return err
		}
	}

	return nil
}

// AppRole SecretID lookup
func (d *vtkData) approleSecretIDLookup() error {
	// Params for SecretID lookup
	optionsLookupSecretID := map[string]interface{}{
		"secret_id": d.approleSecretID,
	}
	lookupAuthPath := fmt.Sprintf("auth/%s/role/%s/secret-id/lookup", authMethod, d.approleName)

	// Get AppRole SecretID params
	lookupSecretID, err := d.vaultClient.Logical().Write(lookupAuthPath, optionsLookupSecretID)
	if err != nil {
		return err
	}
	d.approleSecretIDTTL = make(map[string]int64)
	expirationTime, err := time.Parse(time.RFC3339, lookupSecretID.Data["creation_time"].(string))
	if err != nil {
		return err
	}
	secretIDTTL, _ := strconv.ParseInt(fmt.Sprintf("%s", lookupSecretID.Data["secret_id_ttl"]), 10, 64)
	d.approleSecretIDTTL["creation_time"] = expirationTime.Unix()
	d.approleSecretIDTTL["secret_id_ttl"] = secretIDTTL

	return nil
}

// AppRole Secret ID rotation
func (d *vtkData) approleSecretIDRotation() {
	approleSecretIDLookupRetry := 60
	glog.Infoln("AppRole Secret ID rotation enabled")

	for {
		if err := d.approleSecretIDLookup(); err != nil {
			glog.Errorln(err)
			glog.Errorln("AppRole Secret ID rotation wasn't enabled due to error during getting 'creation_time' for 'secret_id'")
			glog.Errorln("Next retry will be in '" + strconv.Itoa(approleSecretIDLookupRetry) + "' seconds")
			authApproleSecretID.WithLabelValues("rotation-status").Set(0)
			time.Sleep(time.Duration(approleSecretIDLookupRetry) * time.Second)
			approleSecretIDLookupRetry = approleSecretIDLookupRetry * 2
		} else {
			approleSecretIDLookupRetry = 60
			authApproleSecretID.WithLabelValues("rotation-status").Set(1)

			// Set correct value for AppRole SecretID interval
			if approleSecretIDRotationInterval == -1 {
				approleSecretIDRotationInterval = int(float64(d.approleSecretIDTTL["secret_id_ttl"]) * 0.7)
				glog.Infoln("'APPROLE_SECRETID_ROTATION_INTERVAL' wasn't defined, therefore it calculated and set to '" + strconv.Itoa(approleSecretIDRotationInterval) + "' seconds")
			} else if approleSecretIDRotationInterval >= int(d.approleSecretIDTTL["secret_id_ttl"]) {
				approleSecretIDRotationInterval = int(float64(d.approleSecretIDTTL["secret_id_ttl"]) * 0.7)
				glog.Infoln("'APPROLE_SECRETID_ROTATION_INTERVAL' has incorrect value (>= secret_id_ttl), therefore it calculated and set to '" + strconv.Itoa(approleSecretIDRotationInterval) + "' seconds")
			}

			timeWaitBeforeRotation := approleSecretIDRotationInterval - int(time.Now().Unix()-d.approleSecretIDTTL["creation_time"])
			glog.V(2).Infoln("Secret ID will be rotated in '" + strconv.Itoa(timeWaitBeforeRotation) + "' seconds")
			authApproleSecretID.WithLabelValues("next-rotation-timestamp").Set(float64(time.Now().Unix() + int64(timeWaitBeforeRotation)))
			timer := time.NewTimer(time.Duration(timeWaitBeforeRotation) * time.Second)
			<-timer.C
			for {
				glog.V(2).Infoln("Rotating Secret ID...")
				// Params for creating new 'secret_id'
				options := map[string]interface{}{}
				authPath := fmt.Sprintf("auth/%s/role/%s/secret-id", authMethod, d.approleName)

				glog.V(2).Infoln("Generating new Secret ID")
				approleSecretID, err := d.vaultClient.Logical().Write(authPath, options)
				if err != nil {
					glog.Errorln(err)
					glog.Errorln("Waiting 60 seconds before retry of generating new Secret ID")
					authApproleSecretID.WithLabelValues("last-rotation-status").Set(0)
					time.Sleep(60 * time.Second)
				} else {
					d.approleSecretID = approleSecretID.Data["secret_id"]
					// Revoke old 'secret_id' before replace it by new one
					if err := d.approleRevokeOldSecretID(); err != nil {
						glog.Errorln(err)
						authApproleSecretID.WithLabelValues("error-revoke-secret-id").Set(1)
					} else {
						authApproleSecretID.WithLabelValues("error-revoke-secret-id").Set(0)
					}
					// Save 'secret_id' to k8s secret object
					if err := d.updateAppSystemSecret(); err != nil {
						glog.Errorln(err)
						glog.Errorln("Waiting 60 seconds before retry of generating new Secret ID")
						authApproleSecretID.WithLabelValues("last-rotation-status").Set(0)
						time.Sleep(60 * time.Second)
						continue
					}
					glog.V(2).Infoln("Secret ID successfully rotated")
					authApproleSecretID.WithLabelValues("last-rotation-status").Set(1)
					break
				}
			}
		}
	}
}

// Token rotation
func (d *vtkData) tokenRotation() {
	glog.Infoln("Token rotation enabled")
	for {
		timeWaitBeforeRotation := tokenRotationInterval - int(time.Now().Unix()-d.vaultTokenTTL["creation_time"])
		glog.V(2).Infoln("Token will be rotated in '" + strconv.Itoa(timeWaitBeforeRotation) + "' seconds")
		authToken.WithLabelValues("next-rotation-timestamp").Set(float64(time.Now().Unix() + int64(timeWaitBeforeRotation)))
		timer := time.NewTimer(time.Duration(timeWaitBeforeRotation) * time.Second)
		<-timer.C
		glog.V(2).Infoln("Rotating Token...")
		if err := d.approleGetToken(); err != nil {
			glog.Errorln(err)
			glog.Errorln("Waiting 60 seconds before retry generating new token")
			authToken.WithLabelValues("last-rotation-status").Set(0)
			time.Sleep(60 * time.Second)
		} else {
			// Revoke old 'token' before replace it by new one
			if err := d.revokeOldToken(); err != nil {
				glog.Errorln(err)
				authToken.WithLabelValues("error-revoke-token").Set(1)
			} else {
				authToken.WithLabelValues("error-revoke-token").Set(0)
			}
			// Save 'token_accessor' to k8s secret object
			if err := d.updateAppSystemSecret(); err != nil {
				glog.Errorln(err)
				authToken.WithLabelValues("error-save-token-accessor-in-k8s-secret").Set(1)
			} else {
				authToken.WithLabelValues("error-save-token-accessor-in-k8s-secret").Set(0)
			}
			glog.V(2).Infoln("Token successfully rotated")
			authToken.WithLabelValues("last-rotation-status").Set(1)
		}
	}
}
