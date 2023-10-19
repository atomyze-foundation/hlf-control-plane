package main

//go:generate bin/buf generate proto -o proto
//go:generate bin/swagger generate client -f proto/plane.swagger.json -t test/integration
