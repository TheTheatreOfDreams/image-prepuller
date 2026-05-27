package agent

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
)

type Runtime interface {
	PullImage(ctx context.Context, image string) error
	ListImages(ctx context.Context) (map[string]struct{}, error)
	RemoveImage(ctx context.Context, image string) error
	Close() error
}

type CRIRuntime struct {
	conn   *grpc.ClientConn
	client runtimeapi.ImageServiceClient
}

func NewCRIRuntime(ctx context.Context, endpoint string) (*CRIRuntime, error) {
	target := strings.TrimPrefix(endpoint, "unix://")
	conn, err := grpc.DialContext(ctx, "unix://"+target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("connect to CRI endpoint %q: %w", endpoint, err)
	}
	return &CRIRuntime{
		conn:   conn,
		client: runtimeapi.NewImageServiceClient(conn),
	}, nil
}

func (r *CRIRuntime) PullImage(ctx context.Context, image string) error {
	_, err := r.client.PullImage(ctx, &runtimeapi.PullImageRequest{
		Image: &runtimeapi.ImageSpec{Image: image},
	})
	if err != nil {
		return fmt.Errorf("pull image %q: %w", image, err)
	}
	return nil
}

func (r *CRIRuntime) ListImages(ctx context.Context) (map[string]struct{}, error) {
	response, err := r.client.ListImages(ctx, &runtimeapi.ListImagesRequest{})
	if err != nil {
		return nil, fmt.Errorf("list images: %w", err)
	}

	images := map[string]struct{}{}
	for _, image := range response.Images {
		if image.Id != "" {
			images[image.Id] = struct{}{}
		}
		for _, tag := range image.RepoTags {
			images[tag] = struct{}{}
		}
		for _, digest := range image.RepoDigests {
			images[digest] = struct{}{}
		}
	}
	return images, nil
}

func (r *CRIRuntime) RemoveImage(ctx context.Context, image string) error {
	_, err := r.client.RemoveImage(ctx, &runtimeapi.RemoveImageRequest{
		Image: &runtimeapi.ImageSpec{Image: image},
	})
	if err != nil {
		return fmt.Errorf("remove image %q: %w", image, err)
	}
	return nil
}

func (r *CRIRuntime) Close() error {
	if r.conn == nil {
		return nil
	}
	return r.conn.Close()
}
