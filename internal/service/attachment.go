package service

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	forum "stepik.leoscode.http/internal/gen/api"
)


type AttachmentService struct{
	attachments map[int64]forum.Attachment
	threadService *ThreadService
	nextID int
	mu sync.Mutex
}

func NewAttachmentService() *AttachmentService{
	return &AttachmentService{
		attachments: make(map[int64]forum.Attachment),
		nextID: 1,
	}
}


func (a *AttachmentService) UploadAttachment(threadID string, userID openapi_types.UUID, file string) (forum.Attachment, error){
	a.mu.Lock()
	defer a.mu.Unlock()

	threadIDInt, err := strconv.ParseInt(threadID, 10, 64)
	if err != nil{
		return forum.Attachment{}, fmt.Errorf("invalid thread_id: %w", err)
	}
	entry, exists := a.threadService.threads[threadIDInt]
	if !exists{
		return forum.Attachment{}, errors.New("thread not found")
	}
	if entry.AuthorId != userID{
		return forum.Attachment{}, errors.New("user mismatch: cannot modify another user's thread")
	}




	return forum.Attachment{}, nil
}


func (a *AttachmentService) createNewAttachment(filename string, threadID int64) forum.Attachment {
	attachment := forum.Attachment{
		CreatedAt: time.Now(),
		Filename: filename,
		
	}
}