module DockerSocketExample

go 1.17

require (
	github.com/docker/docker v20.10.7+incompatible // https://github.com/docker/ee-engine (see replace section below)
	github.com/sirupsen/logrus v1.8.1
)

require (
	github.com/Microsoft/go-winio v0.5.1 // indirect
	github.com/docker/distribution v2.8.1+incompatible // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-units v0.4.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/go-cmp v0.5.6 // indirect
	github.com/moby/term v0.0.0-20210619224110-3f7ff695adc6 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.3-0.20211202183452-c5a74bcca799 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/stretchr/testify v1.7.0 // indirect
	golang.org/x/net v0.0.0-20220127200216-cd36cc0744dd // indirect
	golang.org/x/sys v0.0.0-20220412211240-33da011f77ad // indirect
	golang.org/x/time v0.0.0-20220609170525-579cf78fd858 // indirect
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
	gotest.tools/v3 v3.3.0 // indirect
)

replace github.com/docker/docker v20.10.7+incompatible => github.com/Mirantis/ee-engine v0.0.0-20220609063157-72f5f55253f3
