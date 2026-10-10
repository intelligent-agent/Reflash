import axios from "axios";

// A config archive from this computer onto the USB drive, by the same calls the
// image upload uses: the board names where it goes by what is in the file (#184).
// Archives are small, so this is one chunk at a time with a few retries; the
// page's image upload keeps its own, longer, patience for images.
const CHUNK = 3 * 1024 * 1024;

export async function uploadArchive(file, onProgress) {
  await axios.put(`/api/upload_start`, { filename: file.name, size: file.size, start_time: Date.now() });
  try {
    for (let offset = 0; offset < file.size || offset === 0; offset += CHUNK) {
      let tries = 0;
      for (;;) {
        try {
          const res = await axios.post(`/api/upload_chunk`, file.slice(offset, offset + CHUNK), {
            headers: { "Content-Type": "application/octet-stream" },
            timeout: 20000,
          });
          if (!res.data.success) throw new Error("the board refused the data");
          break;
        } catch (err) {
          if (++tries > 3) throw err;
          await new Promise((r) => setTimeout(r, 1000 * tries));
        }
      }
      if (onProgress) onProgress(Math.min(1, (offset + CHUNK) / Math.max(1, file.size)));
      if (file.size === 0) break;
    }
  } catch (err) {
    await axios.put(`/api/upload_cancel`).catch(() => {});
    throw err;
  }
  await axios.put(`/api/upload_finish`);
}
