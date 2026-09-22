package secrets

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// deploymentFrameMaxBytes bounds a single length-prefixed frame.
const deploymentFrameMaxBytes = DeploymentMaxFrameBytes

var errDeploymentFrame = errors.New("deployment secret frame violation")

// WriteDeploymentFrame writes a four-byte big-endian length followed by the
// payload bytes. Payloads must be non-empty and bounded.
func WriteDeploymentFrame(writer io.Writer, payload []byte) error {
	if len(payload) == 0 || len(payload) > deploymentFrameMaxBytes {
		return fmt.Errorf("%w: payload length is out of bounds", errDeploymentFrame)
	}
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(payload)))
	if _, err := writer.Write(header); err != nil {
		return fmt.Errorf("write frame header: %w", err)
	}
	if _, err := writer.Write(payload); err != nil {
		return fmt.Errorf("write frame payload: %w", err)
	}
	return nil
}

// ReadDeploymentFrame reads exactly one bounded frame.
func ReadDeploymentFrame(reader io.Reader) ([]byte, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(reader, header); err != nil {
		return nil, fmt.Errorf("read frame header: %w", err)
	}
	length := binary.BigEndian.Uint32(header)
	if length == 0 || length > deploymentFrameMaxBytes {
		return nil, fmt.Errorf("%w: declared length is out of bounds", errDeploymentFrame)
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, fmt.Errorf("read frame payload: %w", err)
	}
	return payload, nil
}

// WriteDeploymentSignedRequest writes the fixed provision/revoke frame shape:
// envelope length, envelope, signature length, signature bytes.
func WriteDeploymentSignedRequest(writer io.Writer, envelope []byte, signature string) error {
	if len(signature) != DeploymentSignatureLength {
		return fmt.Errorf("%w: signature length is invalid", errDeploymentFrame)
	}
	if _, err := base64.RawStdEncoding.DecodeString(signature); err != nil {
		return fmt.Errorf("%w: signature is not canonical base64", errDeploymentFrame)
	}
	if err := WriteDeploymentFrame(writer, envelope); err != nil {
		return err
	}
	return WriteDeploymentFrame(writer, []byte(signature))
}

// ReadDeploymentSignedRequest reads exactly one signed request frame and
// requires EOF at the declared boundary.
func ReadDeploymentSignedRequest(reader io.Reader) ([]byte, string, error) {
	envelope, err := ReadDeploymentFrame(reader)
	if err != nil {
		return nil, "", err
	}
	signature, err := ReadDeploymentFrame(reader)
	if err != nil {
		return nil, "", err
	}
	if len(signature) != DeploymentSignatureLength {
		return nil, "", fmt.Errorf("%w: signature length is invalid", errDeploymentFrame)
	}
	if err := RequireDeploymentEOF(reader); err != nil {
		return nil, "", err
	}
	return envelope, string(signature), nil
}

// RequireDeploymentEOF rejects any trailing bytes after the final frame.
func RequireDeploymentEOF(reader io.Reader) error {
	var trailing [1]byte
	count, err := reader.Read(trailing[:])
	if count > 0 {
		return fmt.Errorf("%w: trailing bytes after frame", errDeploymentFrame)
	}
	if err == nil || errors.Is(err, io.EOF) {
		return nil
	}
	return fmt.Errorf("%w: %v", errDeploymentFrame, err)
}
