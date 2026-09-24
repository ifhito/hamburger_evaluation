package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/photo"
)

// decodeMCPPhoto は転送形式だけを解釈し、画像の検査・変換は HTTP 投稿と共通の処理へ渡す。
// 全体をデコードしてから画像を読むことで、画像の後ろの壊れた Base64 も拒否する。
func decodeMCPPhoto(ctx context.Context, encoded *string) (*photo.Processed, *apiMessage, error) {
	if encoded == nil {
		return nil, nil, nil
	}
	if len(*encoded) > base64.StdEncoding.EncodedLen(int(domain.MaxPhotoBytes)) {
		return nil, &msgPhotoTooLarge, nil
	}
	data, err := base64.StdEncoding.Strict().DecodeString(*encoded)
	if err != nil || len(data) == 0 {
		return nil, &msgPhotoUnsupported, nil
	}
	if int64(len(data)) > domain.MaxPhotoBytes {
		return nil, &msgPhotoTooLarge, nil
	}
	processed, err := photo.Process(ctx, bytes.NewReader(data))
	switch {
	case errors.Is(err, photo.ErrDimensionsTooLarge):
		return nil, &msgPhotoDimensions, nil
	case errors.Is(err, photo.ErrHEIFNotSupported):
		return nil, &msgPhotoHEIF, nil
	case errors.Is(err, photo.ErrUnsupportedImage):
		return nil, &msgPhotoUnsupported, nil
	case err != nil:
		return nil, nil, err
	default:
		return &processed, nil, nil
	}
}
