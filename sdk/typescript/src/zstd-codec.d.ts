declare module 'zstd-codec' {
  export class ZstdCodec {
    static run(callback: (zstd: ZstdModule) => void): void;
  }

  interface ZstdModule {
    Simple: new () => ZstdSimple;
  }

  interface ZstdSimple {
    compress(data: Uint8Array, compressionLevel?: number): Uint8Array;
    decompress(data: Uint8Array): Uint8Array;
  }
}
