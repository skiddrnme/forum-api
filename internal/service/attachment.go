package service

import (
	"errors"
	"fmt"
	"mime/multipart"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
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

func (a *AttachmentService) UploadAttachment(threadID string, userID openapi_types.UUID, file multipart.File, header *multipart.FileHeader) (forum.Attachment, error) {
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

	filename := header.Filename

	attachment := a.createNewAttachment(filename, threadIDInt, header) 
	a.attachments[attachment.Id] = attachment

	return attachment, nil
}

func (a *AttachmentService) createNewAttachment(filename string, threadID int64, header *multipart.FileHeader) forum.Attachment {
	attachment := forum.Attachment{
		Id: openapi_types.UUID(uuid.New()),
		CreatedAt: time.Now(),
		Filename:  filename,
		MimeType:  forum.AttachmentMimeType(header.Header.Get("Content-Type")),
		Size:      header.Size,
		ThreadId:  threadID,
		Url:       nil,
	}
	return attachment
}

func (a *AttachmentService) GetAttachmentMeta(attachment_id openapi_types.UUID) (forum.Attachment, error){
	attachment, ok := a.attachments[attachment_id]
	if !ok{
		return forum.Attachment{}, errors.New("attachment not found")
	}
	return attachment, nil
}
