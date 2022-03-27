# Set a sensible default for the $GOPATH in case it's not exported.
# If you're seeing path errors, try exporting your GOPATH.
ifeq ($(origin GOPATH), undefined)
	GOPATH := $(HOME)/Go
endif

all: mocks

mocks: clean-mocks
# This roundabout call to 'go generate' allows us to:
#  - use modules
#  - prevent grep missing (totally fine) from causing nonzero exit
#  - mirror the pkg/ structure under internal/test/mock
	@find . -name '*.go' | xargs -I{} grep -l '//go:generate' {} | xargs -I{} -P 10 go generate {}

clean-mocks:
	@find . -name 'mock_*.go' | xargs -I{} rm {}

capnp: capnp-boot capnp-pulse capnp-casm capnp-survey
# N.B.:  compiling capnp schemas requires having capnproto.org/go/capnp installed
#		 on the GOPATH.

clean-capnp: clean-capnp-boot clean-capnp-pulse clean-capnp-casm clean-capnp-survey

capnp-boot:
	@mkdir -p internal/api/boot
	@capnp compile -I$(GOPATH)/src/capnproto.org/go/capnp/std -ogo:internal/api/boot --src-prefix=api/ api/boot.capnp

clean-capnp-boot:
	@rm -rf internal/api/boot

capnp-pulse:  clean-capnp-pulse
	@mkdir -p internal/api/pulse
	@capnp compile -I$(GOPATH)/src/capnproto.org/go/capnp/std -ogo:internal/api/pulse --src-prefix=api/ api/pulse.capnp

clean-capnp-pulse:
	@rm -rf internal/api/pulse

capnp-pex:  clean-capnp-pex
	@mkdir -p internal/api/pex
	@capnp compile -I$(GOPATH)/src/capnproto.org/go/capnp/std -ogo:internal/api/pex --src-prefix=api/ api/pex.capnp

clean-capnp-pex:
	@rm -rf internal/api/pex
