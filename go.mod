module github.com/uswitch/vault-webhook

go 1.13

require (
	github.com/alecthomas/template v0.0.0-20160405071501-a0175ee3bccc // indirect
	github.com/alecthomas/units v0.0.0-20151022065526-2efee857e7cf // indirect
	github.com/beorn7/perks v0.0.0-20180321164747-3a771d992973 // indirect
	github.com/json-iterator/go v0.0.0-20180722035151-10a568c51178 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.1 // indirect
	github.com/prometheus/client_golang v0.9.0-pre1.0.20180808080507-5b23715facde
	github.com/prometheus/client_model v0.0.0-20180712105110-5c3871d89910 // indirect
	github.com/prometheus/common v0.0.0-20180801064454-c7de2306084e // indirect
	github.com/prometheus/procfs v0.0.0-20180725123919-05ee40e3a273 // indirect
	github.com/sirupsen/logrus v1.0.4-0.20170822132746-89742aefa4b2
	gopkg.in/airbrake/gobrake.v2 v2.0.9 // indirect
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/gemnasium/logrus-airbrake-hook.v2 v2.1.2 // indirect
	k8s.io/api v0.15.12
	k8s.io/apimachinery v0.15.12
	k8s.io/client-go v0.15.12
	k8s.io/code-generator v0.15.12
	k8s.io/sample-controller v0.0.0-00010101000000-000000000000
)

replace (
	k8s.io/api => k8s.io/api v0.15.12
	k8s.io/apimachinery => k8s.io/apimachinery v0.15.12
	k8s.io/client-go => k8s.io/client-go v0.15.12
	k8s.io/code-generator => k8s.io/code-generator v0.15.12
	k8s.io/sample-controller => k8s.io/sample-controller v0.15.12
)
