package v1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/osbuild/image-builder/internal/tutils"
)

func TestIdentity(t *testing.T) {
	// note: any url will work, it'll only try to contact the osbuild-composer
	// instance when calling /compose or /compose/$uuid
	srv, tokenSrv := startServer(t, "", "")
	defer func() {
		err := srv.Shutdown(context.Background())
		require.NoError(t, err)
	}()
	defer tokenSrv.Close()

	t.Run("VerifyIdentityHeaderMissing", func(t *testing.T) {
		respStatusCode, body := tutils.GetResponseBody(t, "http://localhost:8086/api/image-builder/v1/version", nil)
		require.Equal(t, 400, respStatusCode)
		require.Contains(t, body, "missing x-rh-identity header")
	})

	t.Run("Valid authstring", func(t *testing.T) {
		respStatusCode, _ := tutils.GetResponseBody(t, "http://localhost:8086/api/image-builder/v1/version", &tutils.AuthString0)
		require.Equal(t, 200, respStatusCode)
	})

	t.Run("Valid authstring without entitlements", func(t *testing.T) {
		respStatusCode, _ := tutils.GetResponseBody(t, "http://localhost:8086/api/image-builder/v1/version", &tutils.AuthString0WithoutEntitlements)
		require.Equal(t, 200, respStatusCode)
	})

	t.Run("BogusAuthString", func(t *testing.T) {
		auth := "notbase64"
		respStatusCode, body := tutils.GetResponseBody(t, "http://localhost:8086/api/image-builder/v1/version", &auth)
		require.Equal(t, 400, respStatusCode)
		require.Contains(t, body, "unable to b64 decode x-rh-identity header")
	})

	t.Run("BogusBase64AuthString", func(t *testing.T) {
		auth := "dGhpcyBpcyBkZWZpbml0ZWx5IG5vdCBqc29uCg=="
		respStatusCode, body := tutils.GetResponseBody(t, "http://localhost:8086/api/image-builder/v1/version", &auth)
		require.Equal(t, 400, respStatusCode)
		require.Contains(t, body, "does not contain valid JSON")
	})

	t.Run("EmptyAccountNumber", func(t *testing.T) {
		// AccoundNumber equals ""
		auth := tutils.GetCompleteBase64Header("000000")
		respStatusCode, _ := tutils.GetResponseBody(t, "http://localhost:8086/api/image-builder/v1/version", &auth)
		require.Equal(t, 200, respStatusCode)
	})

	t.Run("EmptyOrgID", func(t *testing.T) {
		// OrgID equals ""
		auth := tutils.GetCompleteBase64Header("")
		respStatusCode, body := tutils.GetResponseBody(t, "http://localhost:8086/api/image-builder/v1/version", &auth)
		require.Equal(t, 400, respStatusCode)
		require.Contains(t, body, "invalid or missing org_id")
	})
}
