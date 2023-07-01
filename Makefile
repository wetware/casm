# Set a sensible default for the $GOPATH in case it's not exported.
# If you're seeing path errors, try exporting your GOPATH.
ifeq ($(origin GOPATH), undefined)
	GOPATH := $(HOME)/Go
endif

all: mocks capnp

mocks: clean-mocks
# This roundabout call to 'go generate' allows us to:
#  - use modules
#  - prevent grep missing (totally fine) from causing nonzero exit
#  - mirror the pkg/ structure under internal/test/mock
	@find . -name '*.go' | xargs -I{} grep -l '//go:generate' {} | xargs -I{} -P 10 gotip generate {}

clean-mocks:
	@find . -name 'mock_*.go' | xargs -I{} rm {}

capnp: capnp-boot capnp-pex capnp-routing capnp-testing capnp-debug
# N.B.:  compiling capnp schemas requires having capnproto.org/go/capnp installed
#		 on the GOPATH.

clean-capnp: clean-capnp-boot clean-capnp-pex clean-capnp-routing clean-capnp-testing clean-capnp-debug

capnp-boot:
	@mkdir -p internal/api/boot
	@capnp compile -I$(GOPATH)/src/capnproto.org/go/capnp/std -ogo:internal/api/boot --src-prefix=api/ api/boot.capnp

clean-capnp-boot:
	@rm -rf internal/api/boot

capnp-routing:  clean-capnp-routing
	@mkdir -p internal/api/routing
	@capnp compile -I$(GOPATH)/src/capnproto.org/go/capnp/std -ogo:internal/api/routing --src-prefix=api/ api/routing.capnp

clean-capnp-routing:
	@rm -rf internal/api/routing

capnp-pex:  clean-capnp-pex
	@mkdir -p internal/api/pex
	@capnp compile -I$(GOPATH)/src/capnproto.org/go/capnp/std -ogo:internal/api/pex --src-prefix=api/ api/pex.capnp

clean-capnp-pex:
	@rm -rf internal/api/pex

capnp-testing:
	@mkdir -p internal/api/testing
	@capnp compile -I$(GOPATH)/src/capnproto.org/go/capnp/std -ogo:internal/api/testing --src-prefix=api/ api/testing.capnp

clean-capnp-testing:
	@rm -rf internal/api/testing

capnp-debug:
	@mkdir -p internal/api/debug
	@capnp compile -I$(GOPATH)/src/capnproto.org/go/capnp/std -ogo:internal/api/debug --src-prefix=api/ api/debug.capnp

clean-capnp-debug:
	@rm -rf internal/api/debug
