fn main() {
    prost_build::compile_protos(&["proto/protomcp.proto"], &["proto/"]).unwrap();
}
