package cloud

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"

	cloudinary "github.com/cloudinary/cloudinary-go/v2"
	"github.com/cloudinary/cloudinary-go/v2/api/uploader"
)

type Cloud struct {
	cld *cloudinary.Cloudinary
}

func New(cloudinaryURL string) (*Cloud, error) {
	c, err := cloudinary.NewFromURL(cloudinaryURL)
	if err != nil { return nil, err }
	return &Cloud{cld: c}, nil
}

func publicID(domain, src string) string {
	h := sha1.Sum([]byte(src))
	return fmt.Sprintf("favget/%s/%s", domain, hex.EncodeToString(h[:]))
}

func (c *Cloud) UploadRemote(ctx context.Context, domain, srcURL string) (string, error) {
	overwrite := true
	resp, err := c.cld.Upload.Upload(ctx, srcURL, uploader.UploadParams{
		PublicID: publicID(domain, srcURL),
		Overwrite: &overwrite,
	})
	if err != nil { return "", err }
	return resp.SecureURL, nil
}
