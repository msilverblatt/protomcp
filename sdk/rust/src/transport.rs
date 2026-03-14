use prost::Message;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::UnixStream;
use crate::proto;

#[derive(Clone)]
pub struct Transport {
    inner: std::sync::Arc<tokio::sync::Mutex<UnixStream>>,
}

impl Transport {
    pub async fn connect(socket_path: &str) -> std::io::Result<Self> {
        let stream = UnixStream::connect(socket_path).await?;
        Ok(Self {
            inner: std::sync::Arc::new(tokio::sync::Mutex::new(stream)),
        })
    }

    pub async fn send(&self, env: &proto::Envelope) -> std::io::Result<()> {
        let data = env.encode_to_vec();
        let length = (data.len() as u32).to_be_bytes();
        let mut stream = self.inner.lock().await;
        stream.write_all(&length).await?;
        stream.write_all(&data).await?;
        stream.flush().await?;
        Ok(())
    }

    /// Send a RawHeader envelope followed by raw payload bytes.
    /// If the payload exceeds PROTOMCP_COMPRESS_THRESHOLD (default 64KB),
    /// it is zstd-compressed before sending.
    pub async fn send_raw(&self, request_id: &str, field_name: &str, data: &[u8]) -> std::io::Result<()> {
        let threshold: usize = std::env::var("PROTOMCP_COMPRESS_THRESHOLD")
            .ok()
            .and_then(|v| v.parse().ok())
            .unwrap_or(65536);

        let (payload, compression, uncompressed_size) = if data.len() > threshold {
            let compressed = zstd::encode_all(std::io::Cursor::new(data), 3)
                .map_err(std::io::Error::other)?;
            (compressed, "zstd".to_string(), data.len() as u64)
        } else {
            (data.to_vec(), String::new(), 0u64)
        };

        let header = proto::Envelope {
            msg: Some(proto::envelope::Msg::RawHeader(proto::RawHeader {
                request_id: request_id.to_string(),
                field_name: field_name.to_string(),
                size: payload.len() as u64,
                compression,
                uncompressed_size,
            })),
            ..Default::default()
        };
        let header_bytes = header.encode_to_vec();
        let length = (header_bytes.len() as u32).to_be_bytes();

        let mut buf = Vec::with_capacity(4 + header_bytes.len() + payload.len());
        buf.extend_from_slice(&length);
        buf.extend_from_slice(&header_bytes);
        buf.extend_from_slice(&payload);

        let mut stream = self.inner.lock().await;
        stream.write_all(&buf).await?;
        stream.flush().await?;
        Ok(())
    }

    pub async fn recv(&self) -> std::io::Result<proto::Envelope> {
        let mut length_buf = [0u8; 4];
        let mut stream = self.inner.lock().await;
        stream.read_exact(&mut length_buf).await?;
        let length = u32::from_be_bytes(length_buf) as usize;
        let mut data = vec![0u8; length];
        stream.read_exact(&mut data).await?;
        proto::Envelope::decode(&data[..])
            .map_err(|e| std::io::Error::new(std::io::ErrorKind::InvalidData, e))
    }
}
