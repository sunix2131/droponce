package application

import (
	"sync"

	"droponce/internal/infrastructure/qr"
)

type RuntimeTransfer struct {
	TransferID string
	RawToken   string
	ShareURL   string
	CancelURL  string
	QRCodePNG  []byte

	mu            *sync.Mutex
	IsDownloading bool
}

type Registry struct {
	mu      sync.RWMutex
	byID    map[string]RuntimeTransfer
	byToken map[string]string
}

func NewRegistry() *Registry {
	return &Registry{byID: map[string]RuntimeTransfer{}, byToken: map[string]string{}}
}

func (r *Registry) Add(t RuntimeTransfer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t.mu == nil {
		t.mu = &sync.Mutex{}
	}
	t.QRCodePNG = append([]byte(nil), t.QRCodePNG...)
	r.byID[t.TransferID] = t
	r.byToken[t.RawToken] = t.TransferID
}

func (r *Registry) Get(id string) (RuntimeTransfer, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.byID[id]
	if !ok {
		return RuntimeTransfer{}, false
	}
	t.QRCodePNG = append([]byte(nil), t.QRCodePNG...)
	return t, true
}

func (r *Registry) GetByToken(token string) (RuntimeTransfer, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.byToken[token]
	if !ok {
		return RuntimeTransfer{}, false
	}
	t := r.byID[id]
	t.QRCodePNG = append([]byte(nil), t.QRCodePNG...)
	return t, true
}

func (r *Registry) Remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.byID[id]
	if ok {
		delete(r.byToken, t.RawToken)
	}
	delete(r.byID, id)
}

func (r *Registry) List() []RuntimeTransfer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]RuntimeTransfer, 0, len(r.byID))
	for _, t := range r.byID {
		t.QRCodePNG = append([]byte(nil), t.QRCodePNG...)
		out = append(out, t)
	}
	return out
}

func (r *Registry) SetDownloading(id string, v bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.byID[id]
	if ok {
		t.IsDownloading = v
		r.byID[id] = t
	}
}

func (r *Registry) SetShareURL(id, shareURL string) error {
	png, err := qr.PNG(shareURL)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.byID[id]
	if !ok {
		return nil
	}
	t.ShareURL = shareURL
	t.QRCodePNG = png
	r.byID[id] = t
	return nil
}
