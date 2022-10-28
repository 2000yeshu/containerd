//go:build windows

package ociwclayer

import (
	"archive/tar"
	"context"
	"io"
	"path/filepath"

	"github.com/Microsoft/go-winio/backuptar"
	"github.com/Microsoft/hcsshim/internal/wclayer"
)

// ExportLayerToTar writes an OCI layer tar stream from the provided on-disk layer.
// The caller must specify the parent layers, if any, ordered from lowest to
// highest layer.
//
// The layer will be mounted for this process, so the caller should ensure that
// it is not currently mounted.
func ExportLayerToTar(ctx context.Context, w io.Writer, path string, parentLayerPaths []string) error {
	err := wclayer.ActivateLayer(ctx, path)
	if err != nil {
		return err
	}
	defer func() {
		_ = wclayer.DeactivateLayer(ctx, path)
	}()

	// Prepare and unprepare the layer to ensure that it has been initialized.
	err = wclayer.PrepareLayer(ctx, path, parentLayerPaths)
	if err != nil {
		return err
	}
	err = wclayer.UnprepareLayer(ctx, path)
	if err != nil {
		return err
	}

	r, err := wclayer.NewLayerReader(ctx, path, parentLayerPaths)
	if err != nil {
		return err
	}

	err = writeTarFromLayer(ctx, r, w)
	cerr := r.Close()
	if err != nil {
		return err
	}
	return cerr
}

func writeTarFromLayer(ctx context.Context, r wclayer.LayerReader, w io.Writer) error {
	t := tar.NewWriter(w)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		name, size, fileInfo, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if fileInfo == nil {
			// Write a whiteout file.
			hdr := &tar.Header{
				Name: filepath.ToSlash(filepath.Join(filepath.Dir(name), whiteoutPrefix+filepath.Base(name))),
			}
			err := t.WriteHeader(hdr)
			if err != nil {
				return err
			}
		} else {
			err = backuptar.WriteTarFileFromBackupStream(t, r, name, size, fileInfo)
			if err != nil {
				return err
			}
		}
	}
	return t.Close()
}