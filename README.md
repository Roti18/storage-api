# Storage API File Manager

A high-performance, self-hosted file manager API written in Go (Fiber). It supports multiple storage mounts (local/SSD/HDD), SQLite-based indexing for fast search, and JWT authentication.

## Database System (SQLite Index)

The application uses an embedded **SQLite** database (`storage_index.db`) to index file metadata. This ensures instant search results and rapid statistics calculation without the need to traverse the filesystem (which can be slow, especially on mechanical HDDs).

- **Automatic Indexing**: runs at startup and every 30 minutes in the background.
- **Real-time Updates**: Write operations (Upload, Rename, Delete, etc.) automatically trigger cache invalidation and re-indexing for the affected storage.
- **Features**: Enables complex queries like "Find all JPGs modified in the last 7 days" instantly.

## API Endpoints

### Public
| Method | Endpoint | Description | Query / Body |
| :--- | :--- | :--- | :--- |
| `GET` | `/ping` | Health check & Latency | - |
| `POST` | `/api/login` | Admin Login | Body: `{"password": "your_password"}` |

### Protected (Requires Bearer Token)
Add header: `Authorization: Bearer <token>`

#### File & Folder Operations
| Method | Endpoint | Description | Query / Body |
| :--- | :--- | :--- | :--- |
| `GET` | `/api/` | List all storage drives | - |
| `GET` | `/api/files` | List files/folders | `?storage=nx1&path=/docs`<br>`&show_hidden=true`<br>`&recursive=true` |
| `GET` | `/api/preview` | Preview file / Stream video | `?storage=nx1&path=/video.mp4`<br>`&thumb=true` (video thumbnail) |
| `GET` | `/api/download` | Force download | `?storage=nx1&path=/file.pdf` |
| `POST` | `/api/upload` | Upload file | `?storage=nx1&path=/dest`<br>Body: Multipart `file` |
| `POST` | `/api/folder` | Create folder | Body: `{"storage": "nx1", "path": "/new_folder"}` |
| `PUT` | `/api/rename` | Rename or Move | Body: `{"storage": "nx1", "old_path": "/src", "new_path": "/dst"}` |
| `POST` | `/api/copy` | Copy file/folder | Body: `{"storage": "nx1", "old_path": "/src", "new_path": "/dst"}` |
| `POST` | `/api/duplicate` | Duplicate file | Body: `{"storage": "nx1", "path": "/file.txt"}` |
| `DELETE` | `/api/delete` | Delete file/folder | `?storage=nx1&path=/trash` |

#### Search & Stats (SQLite Powered)
| Method | Endpoint | Description | Query / Body |
| :--- | :--- | :--- | :--- |
| `GET` | `/api/search` | Fast Search | `?storage=nx1&ext=jpg,png`<br>`&limit=50&offset=0`<br>`&days=7` |
| `GET` | `/api/recent` | Recent Files | `?storage=nx1&limit=20` |
| `POST` | `/api/stats` | File Counts by Category | `?storage=nx1`<br>Body: `{"images": ["jpg", "png"], "videos": ["mp4"], "others": []}` |
| `GET` | `/api/reindex` | Force Re-index | - |

## Deployment & Storage Setup

### 1. Permanent Storage Mounting (Recommended)
For Linux servers, it is highly recommended to use permanent mount points (e.g., `/mnt/ssd`) instead of temporary desktop mounts (`/run/media/...`). This prevents path changes after unexpected reboots or power outages.

A script is provided to automate this:
```bash
cd storages-api
chmod +x setup_ssd.sh
./setup_ssd.sh
```
This script will:
- Add a permanent entry to `/etc/fstab` using the SSD's UUID.
- Create `/mnt/ssd` and mount the drive there.
- Automatically update your `.env` with the new paths.
- Restart the Docker containers.

### 2. Manual Installation
If you prefer manual setup:
1. Copy `.env.example` to `.env`.
2. Configure your storage paths in `.env`.
3. Run the application using Docker:
   ```bash
   docker-compose up -d --build
   ```

## Configuration
Manage settings via `.env`:
```env
APP_PORT=3003
PASSWORD=your_secure_password
JWT_SECRET=your_secret_key
HOST_PATH_SSD=/mnt/ssd
HOST_PATH_HDD=/home/roniserv
STORAGE_MOUNTS=ssd:/mnt/ssd,hdd:/home/roniserv
```
