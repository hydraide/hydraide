module github.com/hydraide/hydraide/docs/tasks/patch-expired-many/smoke

go 1.26.2

require github.com/hydraide/hydraide/sdk/go/hydraidego/v3 v3.0.0

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/vmihailenco/msgpack/v5 v5.4.1 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260226221140-a57be14db171 // indirect
	google.golang.org/grpc v1.81.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace github.com/hydraide/hydraide/sdk/go/hydraidego/v3 => ../../../../sdk/go/hydraidego
