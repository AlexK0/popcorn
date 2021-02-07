package client

import (
	"crypto/sha256"
	"encoding/hex"
	"os"

	pb "github.com/AlexK0/popcorn/internal/api/proto/v1"
	"github.com/AlexK0/popcorn/internal/common"
)

// MakeClientHeaderMeta ...
func MakeClientHeaderMeta(headerPath string) (*pb.HeaderClientMeta, error) {
	headerStat, err := os.Stat(headerPath)
	if err != nil {
		return nil, err
	}
	return &pb.HeaderClientMeta{
		FilePath: headerPath,
		MTime:    headerStat.ModTime().UnixNano(),
	}, nil
}

// MakeHeaderFullData ...
func MakeHeaderFullData(clientMeta *pb.HeaderClientMeta) (*pb.HeaderFullData, error) {
	headerContentBuffer, err := common.ReadFile(clientMeta.FilePath)
	if err != nil {
		return nil, err
	}
	headerBody := headerContentBuffer.Bytes()
	sha256sum := sha256.Sum256(headerBody)
	return &pb.HeaderFullData{
		GlobalMeta: &pb.HeaderGlobalMeta{
			ClientMeta: clientMeta,
			SHA256Sum:  hex.EncodeToString(sha256sum[:]),
		},
		HeaderBody: headerBody,
	}, nil
}
