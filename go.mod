module vault-to-k8s

go 1.12

require (
	github.com/Azure/go-autorest v11.7.1+incompatible
	github.com/evanphx/json-patch v4.2.0+incompatible // indirect
	github.com/gogo/protobuf v1.2.2-0.20190723190241-65acae22fc9d // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/google/uuid v1.1.1 // indirect
	github.com/gregjones/httpcache v0.0.0-20180305231024-9cad4c3443a7 // indirect
	github.com/hashicorp/go-hclog v0.9.2
	github.com/hashicorp/vault v1.3.1
	github.com/hashicorp/vault-plugin-secrets-kv v0.5.2
	github.com/hashicorp/vault/api v1.0.5-0.20191216174727-9d51b36f3ae4
	github.com/hashicorp/vault/sdk v0.1.14-0.20191218020134-06959d23b502
	github.com/namsral/flag v1.7.4-pre
	github.com/pkg/errors v0.8.1
	github.com/prometheus/client_golang v1.2.1
	github.com/prometheus/client_model v0.0.0-20190812154241-14fe0d1b01d4
	github.com/prometheus/common v0.7.0
	github.com/spf13/pflag v1.0.3 // indirect
	k8s.io/api v0.0.0-20191114100237-2cd11237263f
	k8s.io/apimachinery v0.0.0-20191004115701-31ade1b30762
	k8s.io/client-go v0.0.0-20191114101336-8cba805ad12d
	k8s.io/klog v0.4.0 // indirect
	k8s.io/kube-openapi v0.0.0-20190709113604-33be087ad058 // indirect
	k8s.io/utils v0.0.0-20190809000727-6c36bc71fc4a // indirect
)
