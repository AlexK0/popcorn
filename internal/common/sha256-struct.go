package common

import (
	"crypto/sha256"
	"encoding/binary"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/user"

	pb "github.com/AlexK0/popcorn/internal/api/proto/v1"
)

type SHA256Struct struct {
	B0_7, B8_15, B16_23, B24_31 uint64
}

func (h *SHA256Struct) IsEmpty() bool {
	return h.B0_7 == 0 && h.B8_15 == 0 && h.B16_23 == 0 && h.B24_31 == 0
}

func makeSHA256Struct(b []byte) SHA256Struct {
	return SHA256Struct{
		B0_7:   binary.BigEndian.Uint64(b[0:8]),
		B8_15:  binary.BigEndian.Uint64(b[8:16]),
		B16_23: binary.BigEndian.Uint64(b[16:24]),
		B24_31: binary.BigEndian.Uint64(b[24:32]),
	}
}

func MakeSHA256StructFromArray(b [32]byte) SHA256Struct {
	return makeSHA256Struct(b[:])
}

func MakeSHA256StructFromSlice(b []byte) SHA256Struct {
	if len(b) < 32 {
		arr := [32]byte{}
		copy(arr[:], b)
		return makeSHA256Struct(arr[:])
	}
	return makeSHA256Struct(b)
}

func SHA256StructToSHA256Message(sha256struct SHA256Struct) *pb.SHA256Message {
	return &pb.SHA256Message{
		B0_7:   sha256struct.B0_7,
		B8_15:  sha256struct.B8_15,
		B16_23: sha256struct.B16_23,
		B24_31: sha256struct.B24_31,
	}
}

func SHA256MessageToSHA256Struct(sha256message *pb.SHA256Message) SHA256Struct {
	return SHA256Struct{
		B0_7:   sha256message.B0_7,
		B8_15:  sha256message.B8_15,
		B16_23: sha256message.B16_23,
		B24_31: sha256message.B24_31,
	}
}

func feedHash(hasher io.Writer, data []byte) {
	_, _ = hasher.Write(data)
	_, _ = hasher.Write([]byte{0, 1, 2, 2, 3, 3, 3, 4, 4, 4, 4, 5, 5, 5, 5, 5})
}

func MakeUniqueClientID() (string, *pb.SHA256Message, error) {
	netInterfaces, err := net.Interfaces()
	if err != nil {
		return "", nil, err
	}

	hasher := sha256.New()
	for _, netInterface := range netInterfaces {
		if netInterface.Name == "lo" {
			continue
		}
		feedHash(hasher, []byte(netInterface.Name))
		feedHash(hasher, []byte(netInterface.HardwareAddr))
	}

	user, err := user.Current()
	if err != nil {
		return "", nil, err
	}
	feedHash(hasher, []byte(user.Uid))
	feedHash(hasher, []byte(user.Gid))
	feedHash(hasher, []byte(user.Name))
	feedHash(hasher, []byte(user.Username))
	feedHash(hasher, []byte(user.HomeDir))

	machineIDHex, err := ioutil.ReadFile("/etc/machine-id")
	if err != nil {
		return "", nil, err
	}

	feedHash(hasher, machineIDHex)
	return user.Username, SHA256StructToSHA256Message(MakeSHA256StructFromSlice(hasher.Sum(nil))), nil
}

func GetFileSHA256(filePath string) (SHA256Struct, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return SHA256Struct{}, err
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return SHA256Struct{}, err
	}
	return MakeSHA256StructFromSlice(hasher.Sum(nil)), nil
}
