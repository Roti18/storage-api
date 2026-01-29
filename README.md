# Storage API List

## System & Auth (Public)
- GET /
  Ex: http://localhost:3003/
- GET /ping
  Ex: http://localhost:3003/ping (Response include latency)
- POST /api/login
  Body: {"username": "admin", "password": "password123"}
  Ex: curl -X POST http://localhost:3003/api/login -d '{"username":"admin","password":"password123"}' -H "Content-Type: application/json"

## Read Operations (Public)
- GET /api/files
  Ex: http://localhost:3003/api/files?storage=ssd1&path=/documents
- GET /api/preview
  Ex: http://localhost:3003/api/preview?storage=ssd1&path=/images/photo.jpg
- GET /api/download
  Ex: http://localhost:3003/api/download?storage=ssd1&path=/videos/movie.mp4

## Write Operations (Protected)
Auth Header: Authorization: Bearer <token>

- POST /api/folder
  Body: {"storage": "ssd1", "path": "/new_folder"}
  Ex: curl -X POST http://localhost:3003/api/folder -H "Authorization: Bearer <token>" -d '{"storage":"ssd1","path":"/new_folder"}' -H "Content-Type: application/json"
- POST /api/upload
  Ex: curl -X POST "http://localhost:3003/api/upload?storage=ssd1&path=/uploads" -H "Authorization: Bearer <token>" -F "file=@image.png"
- PUT /api/rename
  Body: {"storage": "ssd1", "old_path": "/old.txt", "new_path": "/new.txt"}
  Ex: curl -X PUT http://localhost:3003/api/rename -H "Authorization: Bearer <token>" -d '{"storage":"ssd1","old_path":"/old.txt","new_path":"/new.txt"}' -H "Content-Type: application/json"
- DELETE /api/delete
  Ex: curl -X DELETE "http://localhost:3003/api/delete?storage=ssd1&path=/trash/file.txt" -H "Authorization: Bearer <token>"
