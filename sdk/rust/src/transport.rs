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
