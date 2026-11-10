module github.com/chez-shanpu/tastecheck

go 1.27.2

require (
	github.com/chez-shanpu/typesafeai-go v0.2.0
	github.com/spf13/cobra v1.10.2
	github.com/spf13/pflag v1.1.0
	go.yaml.in/yaml/v3 v3.0.6
	golang.org/x/sync v0.25.0
)

require (
	github.com/BurntSushi/toml v1.6.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	golang.org/x/exp/typeparams v0.0.0-20231108232855-2478ac86f678 // indirect
	golang.org/x/mod v0.42.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/telemetry v0.0.0-20261009151841-97e8ea77aa2a // indirect
	golang.org/x/tools v0.52.0 // indirect
	honnef.co/go/tools v0.8.2 // indirect
	mvdan.cc/gofumpt v0.12.0 // indirect
)

tool (
	golang.org/x/tools/cmd/goimports
	honnef.co/go/tools/cmd/staticcheck
	mvdan.cc/gofumpt
)
