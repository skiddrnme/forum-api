package service

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	forum "stepik.leoscode.http/internal/gen/api"
)

type AttachmentService struct {
	attachments   map[openapi_types.UUID]forum.Attachment
	threadService *ThreadService
	mu            sync.Mutex
}

func NewAttachmentService(threadService *ThreadService) *AttachmentService {
	return &AttachmentService{
		attachments:   make(map[openapi_types.UUID]forum.Attachment),
		threadService: threadService,
	}
}

func (a *AttachmentService) UploadAttachment(threadID string, userID openapi_types.UUID, file string, size int64, mimeType forum.AttachmentMimeType) (forum.Attachment, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	threadIDInt, err := strconv.ParseInt(threadID, 10, 64)
	if err != nil {
		return forum.Attachment{}, fmt.Errorf("invalid thread_id: %w", err)
	}
	entry, exists := a.threadService.threads[threadIDInt]
	if !exists {
		return forum.Attachment{}, errors.New("thread not found")
	}
	if entry.AuthorId != userID {
		return forum.Attachment{}, errors.New("user mismatch: cannot modify another user's thread")
	}

	attachment := a.createNewAttachment(file, threadIDInt, size, mimeType)
	a.attachments[attachment.Id] = attachment

	return attachment, nil
}

func (a *AttachmentService) createNewAttachment(filename string, threadID int64, size int64, mimeType forum.AttachmentMimeType) forum.Attachment {
	attachment := forum.Attachment{
		CreatedAt: time.Now(),
		Filename:  filename,
		MimeType:  mimeType,
		Size:      size,
		ThreadId:  threadID,
		Url:       nil,
	}
	return attachment
}
