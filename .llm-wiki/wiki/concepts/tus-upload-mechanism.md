# TUS Upload Mechanism

Laju Go uses [tusdfiber](https://github.com/maulanashalihin/tusdfiber) for resumable file uploads via the TUS protocol, integrated with Go Fiber natively.

## Architecture

```
Browser (tus-js-client)         Go Fiber Server
        │                             │
        │  POST /tus/files            │
        │  Upload-Length: <size>      │
        ├────────────────────────────►│ tusdfiber.PostFile()
        │  201 Created                │ filestore.NewUpload()
        │  Location: /tus/files/<id>  │
        │◄────────────────────────────┤
        │                             │
        │  PATCH /tus/files/<id>      │
        │  Upload-Offset: 0           │
        │  [chunk data]               │
        ├────────────────────────────►│ tusdfiber.PatchFile()
        │  204 No Content             │ filestore.WriteChunk()
        │  Upload-Offset: <offset>    │
        │◄────────────────────────────┤
        │  (repeat PATCH for chunks)  │
        │                             │
        │  (on final chunk:           │
        │   offset == size)           │
        │                             ├──► processCompletedUploads()
        │                                 │ copy file from
        │                                 │ storage/uploads/<id>
        │                                 │ to storage/completed/<name>
        │                                 │
        │                                 ▼
        │                          /storage/completed/<file>
        │                          (served via app.Static)
```

## Storage Layout

```
storage/
├── uploads/              ← tusd filestore (internal format)
│   ├── <upload-id>       ← raw file data
│   └── <upload-id>.info  ← metadata (size, filename, etc.)
├── completed/            ← post-processed files (original names)
│   └── <original-name>   ← accessible via /storage/completed/<name>
└── avatars/              ← legacy avatar uploads (multipart)
    └── <user>_<ts>.<ext>
```

## Key Endpoints

| Endpoint | Method | Purpose | Auth |
|----------|--------|---------|------|
| `/tus/files` | POST | Create upload | Yes |
| `/tus/files` | OPTIONS | Protocol discovery | Yes (skips OPTIONS) |
| `/tus/files/:id` | HEAD | Get offset/info | Yes |
| `/tus/files/:id` | PATCH | Upload chunk | Yes |
| `/tus/files/:id` | GET | Download file | Yes |
| `/tus/files/:id` | DELETE | Terminate upload | Yes |
| `/storage/completed/<name>` | GET | Download completed file | No (public) |
| `/app/upload` | POST | Avatar upload (legacy) | Yes + CSRF |

## Configuration Requirements

1. **`StreamRequestBody: true`** — Required in `fiber.Config` for streaming large upload bodies.
2. **Notification channels** — `NotifyCreatedUploads/CompleteUploads` must be `false` OR their channels must be drained in a goroutine. Channels are unbuffered, sending blocks until read.
3. **BasePath vs Group** — When registering TUS routes directly on `app` (not a group), `BasePath` must include the full prefix so `Location` URLs are correct (e.g. `/tus/files/`).

## Implementation Files

- `app/handlers/upload.go` — TUS handler setup, post-processing, avatar upload
- `routes/web.go` — Route registration for `/tus/*` and `/app/*`
- `frontend/src/pages/app/UploadTest.svelte` — Upload test page with drag-drop
- `cmd/laju-go/main.go` — `StreamRequestBody` config, storage dirs

## Post-Processing Strategy

When an upload completes via TUS (`NotifyCompleteUploads` event), there are 2 scenarios:

### Scenario A: Direct Copy to `completed/` (current)

Suitable for files that can be downloaded directly without additional processing (documents, images, zip, etc.).

```
storage/uploads/<id>   ← raw file from TUS
       │
       ▼ copy
storage/completed/<filename>   ← access via /storage/completed/<name>
```

**Advantages:**

- Clean URL using the original name
- No auth needed (via `app.Static`)
- Immediately accessible

### Scenario B: Further Processing (transcode, compress, etc.)

Suitable for files that need to be processed first before they can be accessed (video → HLS, image → thumbnail, etc.).

```
storage/uploads/<id>   ← raw file from TUS (do not copy to completed)
       │
       ▼ (CompleteUploads event directly triggers processing)
FFmpeg / ImageMagick / etc.
       │
       ▼
storage/hls/<id>/          ← processing output
├── 1080p/
│   ├── segment-001.ts
│   └── playlist.m3u8
├── 720p/
│   └── ...
└── master.m3u8
```

**Advantages:**

- Saves IO — no need to copy first then process
- Original file stays in `uploads/` — can be deleted after processing is done
- Processing output is stored in a separate folder with its own structure

### When to Choose A vs B

| Scenario | A (completed) | B (direct processing) |
|----------|:-------------:|:---------------------:|
| Document/PDF/Image | ✅ | ❌ |
| Video → HLS | ❌ | ✅ |
| ZIP → extract & store | ❌ | ✅ |
| Large file → compress | ❌ | ✅ |

## Upload Policy: TUS vs Multipart

| Upload | Mechanism | Max Size | Endpoint | Resumable | CSRF | Requirement |
|--------|-----------|----------|----------|-----------|------|-------------|
| **Avatar** (Profile) | Multipart POST | 5MB | `POST /app/upload` | ❌ | ✅ | Update DB + sync session |
| **Large file** (UploadTest) | TUS chunked | 1GB | `POST /tus/files` | ✅ | ❌ | Just store the file |

**Design decision:** Separated because:

1. **TUS overhead is not worth it** — A 100KB avatar needs 3 requests (POST, HEAD, PATCH), multipart only needs 1 POST
2. **Different requirements** — Avatar must update DB `user.avatar` + sync session, needs CSRF. TUS just stores the file
3. **Resumable is only useful for large files** — A 100KB failure just means re-uploading, 500MB is when you lose out without resumable

## Post-Upload Processing

When an upload completes via TUS:

1. `tusdfiber` sends event to `CompleteUploads` channel
2. Goroutine in `processCompletedUploads()` reads the event
3. File is copied from `storage/uploads/<id>` to `storage/completed/<filename>`
4. Filename is extracted from `Upload-Metadata` header (base64-decoded)

This allows direct download via `/storage/completed/<filename>` without authentication. For production, this copy step can be removed and downloads handled via the TUS GET endpoint with auth.
