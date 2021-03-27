all: capnp

capnp: 
# N.B.:  compiling capnp schemas requires having github.com/capnproto/go-capnproto2 installed
#		 on the GOPATH.  Some setup is required.
#
#		  1. cd GOPATH/src/zombiezen.com/go/capnproto2
#		  2. git checkout VERSION_TAG  # See: https://github.com/capnproto/go-capnproto2/releases
#		  3. make capnp
#
	@capnp compile -I$(GOPATH)/src/zombiezen.com/go/capnproto2/std -ogo:internal/mesh --src-prefix=api/ api/mesh.capnp