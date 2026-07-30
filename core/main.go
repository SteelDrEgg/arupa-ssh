package main

// The SSH plugin runtime lives in core; browser assets are built from ../ui.

import arupagrpc "github.com/SteelDrEgg/arupa-sdk/golang/grpc"

// main serves the SSH terminal plugin through the SDK's gRPC runtime.
func main() {
	arupagrpc.Serve(newSSHServer().sdk)
}
