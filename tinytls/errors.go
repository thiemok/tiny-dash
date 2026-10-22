package tinytls

import (
	"errors"
	"strconv"

	"github.com/soypat/lneto/crypto/tlsraw"
)

var (
	ErrNoRand            = errors.New("tinytls: Config.Rand required")
	ErrInsecure          = errors.New("tinytls: Verifier required unless InsecureSkipVerify")
	ErrNoServerName      = errors.New("tinytls: ServerName required to verify the server")
	ErrServerName        = errors.New("tinytls: ServerName too long")
	ErrHelloRetry        = errors.New("tinytls: HelloRetryRequest unsupported")
	ErrMessageTooLarge   = errors.New("tinytls: handshake message exceeds buffer")
	ErrClosed            = errors.New("tinytls: connection closed")
	ErrUnsupportedUpdate = errors.New("tinytls: KeyUpdate unsupported")
	ErrShortHandshakeBuf = errors.New("tinytls: Config.HandshakeBuf shorter than HandshakeBufSize")
)

// AlertError is a protocol violation by the server, answered with this alert.
type AlertError tlsraw.AlertDescription

func (a AlertError) Error() string {
	return "tinytls: sent alert " + alertName(tlsraw.AlertDescription(a))
}

// PeerAlertError is a fatal alert received from the server.
type PeerAlertError tlsraw.AlertDescription

func (a PeerAlertError) Error() string {
	return "tinytls: received alert " + alertName(tlsraw.AlertDescription(a))
}

// VerifyError is the Verifier's rejection of the server's certificate chain,
// name or CertificateVerify signature. It is answered with bad_certificate.
type VerifyError struct{ Err error }

func (v VerifyError) Error() string { return "tinytls: verify server: " + v.Err.Error() }
func (v VerifyError) Unwrap() error { return v.Err }

func alertName(d tlsraw.AlertDescription) string {
	switch d {
	case tlsraw.AlertCloseNotify:
		return "close_notify"
	case tlsraw.AlertUnexpectedMessage:
		return "unexpected_message"
	case tlsraw.AlertBadRecordMAC:
		return "bad_record_mac"
	case tlsraw.AlertRecordOverflow:
		return "record_overflow"
	case tlsraw.AlertHandshakeFailure:
		return "handshake_failure"
	case tlsraw.AlertBadCertificate:
		return "bad_certificate"
	case tlsraw.AlertUnsupportedCertificate:
		return "unsupported_certificate"
	case tlsraw.AlertCertificateExpired:
		return "certificate_expired"
	case tlsraw.AlertCertificateUnknown:
		return "certificate_unknown"
	case tlsraw.AlertIllegalParameter:
		return "illegal_parameter"
	case tlsraw.AlertUnknownCA:
		return "unknown_ca"
	case tlsraw.AlertDecodeError:
		return "decode_error"
	case tlsraw.AlertDecryptError:
		return "decrypt_error"
	case tlsraw.AlertProtocolVersion:
		return "protocol_version"
	case tlsraw.AlertInsufficientSecurity:
		return "insufficient_security"
	case tlsraw.AlertInternalError:
		return "internal_error"
	case tlsraw.AlertUserCanceled:
		return "user_canceled"
	case tlsraw.AlertMissingExtension:
		return "missing_extension"
	case tlsraw.AlertUnsupportedExtension:
		return "unsupported_extension"
	case tlsraw.AlertUnrecognizedName:
		return "unrecognized_name"
	case tlsraw.AlertCertificateRequired:
		return "certificate_required"
	}
	return strconv.Itoa(int(d))
}
