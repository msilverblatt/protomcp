module resources-and-prompts-example

go 1.25.6

require github.com/msilverblatt/protomcp/sdk/go v0.0.0

require (
	github.com/klauspost/compress v1.18.4 // indirect
	github.com/msilverblatt/protomcp v0.0.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace (
	github.com/msilverblatt/protomcp => ../../../
	github.com/msilverblatt/protomcp/sdk/go => ../../../sdk/go
)
