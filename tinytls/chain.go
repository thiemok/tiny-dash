package tinytls

import (
	"encoding/binary"
	"io"

	"github.com/soypat/lneto"
	"github.com/soypat/lneto/crypto/tlsraw"
)

// certChain is the lcrypto.CertChain of a received Certificate message, RFC
// 8446 4.4.2. Its certificates alias the message.
type certChain struct {
	certs [maxPeerCerts][]byte
	n     int
}

func (cc *certChain) parse(body []byte) error {
	*cc = certChain{}
	if len(body) < 4 {
		return AlertError(tlsraw.AlertDecodeError)
	}
	ctxLen := int(body[0])
	body = body[1:]
	if ctxLen != 0 {
		// A server's Certificate answers no CertificateRequest, so its context is empty.
		return AlertError(tlsraw.AlertIllegalParameter)
	}
	listLen := uint24(body)
	body = body[3:]
	if listLen != len(body) {
		return AlertError(tlsraw.AlertDecodeError)
	}
	for len(body) > 0 {
		if cc.n == maxPeerCerts {
			return AlertError(tlsraw.AlertBadCertificate)
		} else if len(body) < 3 {
			return AlertError(tlsraw.AlertDecodeError)
		}
		certLen := uint24(body)
		body = body[3:]
		if certLen == 0 || certLen+2 > len(body) {
			return AlertError(tlsraw.AlertDecodeError)
		}
		cc.certs[cc.n] = body[:certLen]
		cc.n++
		body = body[certLen:]
		extLen := int(binary.BigEndian.Uint16(body))
		body = body[2:]
		if extLen > len(body) {
			return AlertError(tlsraw.AlertDecodeError)
		}
		body = body[extLen:]
	}
	if cc.n == 0 {
		return AlertError(tlsraw.AlertCertificateRequired)
	}
	return nil
}

func (cc *certChain) NumCerts() int { return cc.n }

func (cc *certChain) Cert(dst []byte, i int) (int, error) {
	der, err := cc.CertView(i)
	if err != nil {
		return 0, err
	}
	if len(dst) < len(der) {
		return len(der), io.ErrShortBuffer
	}
	return copy(dst, der), nil
}

func (cc *certChain) CertView(i int) ([]byte, error) {
	if i < 0 || i >= cc.n {
		return nil, lneto.ErrInvalidField
	}
	return cc.certs[i], nil
}
