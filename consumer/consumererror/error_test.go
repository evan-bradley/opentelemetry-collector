// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package consumererror

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var errTest = errors.New("consumererror testing error")

func Test_New(t *testing.T) {
	httpStatus := 500
	grpcStatus := status.New(codes.Aborted, "aborted")
	wantErr := &Error{
		error:      errTest,
		httpStatus: httpStatus,
		grpcStatus: grpcStatus,
	}

	newErr := New(errTest,
		WithHTTPStatus(httpStatus),
		WithGRPCStatus(grpcStatus),
	)

	require.Equal(t, wantErr, newErr)
}

func Test_Error(t *testing.T) {
	newErr := New(errTest)

	require.Equal(t, errTest.Error(), newErr.Error())
}

func TestUnwrap(t *testing.T) {
	err := &Error{
		error: errTest,
	}

	unwrapped := err.Unwrap()

	require.Equal(t, errTest, unwrapped)
}

func TestCombine(t *testing.T) {
	err0 := &Error{}
	err1 := &Error{}
	want := &Error{error: errors.Join(err0, err1, errTest)}

	err := Combine(err0, err1, errTest)

	require.Equal(t, want, err)

}

func TestError_Error(t *testing.T) {
	err := &Error{
		error: errTest,
	}

	require.Equal(t, errTest.Error(), err.Error())
}

func TestError_Unwrap(t *testing.T) {
	err := &Error{
		error: errTest,
	}

	require.Equal(t, []error{errTest}, err.Unwrap())
}

func TestError_HTTPStatus(t *testing.T) {
	serverErr := http.StatusTooManyRequests
	testCases := []struct {
		name       string
		httpStatus int
		grpcStatus *status.Status
		want       int
		hasCode    bool
	}{
		{
			name:       "Passes through HTTP status",
			httpStatus: serverErr,
			want:       serverErr,
			hasCode:    true,
		},
		{
			name:       "Converts gRPC status",
			grpcStatus: status.New(codes.ResourceExhausted, errTest.Error()),
			want:       serverErr,
			hasCode:    true,
		},
		{
			name:       "Passes through HTTP status when gRPC status also present",
			httpStatus: serverErr,
			grpcStatus: status.New(codes.OK, errTest.Error()),
			want:       serverErr,
			hasCode:    true,
		},
		{
			name:    "No statuses set",
			hasCode: false,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			err := Error{
				error:      errTest,
				httpStatus: tt.httpStatus,
				grpcStatus: tt.grpcStatus,
			}

			s, ok := err.HTTPStatus()

			require.Equal(t, tt.hasCode, ok)
			require.Equal(t, tt.want, s)
		})
	}
}

func TestError_GRPCStatus(t *testing.T) {
	httpStatus := http.StatusTooManyRequests
	otherHTTPStatus := http.StatusOK
	serverErr := status.New(codes.ResourceExhausted, errTest.Error())
	testCases := []struct {
		name       string
		httpStatus int
		grpcStatus *status.Status
		want       *status.Status
		hasCode    bool
	}{
		{
			name:       "Converts HTTP status",
			httpStatus: httpStatus,
			want:       serverErr,
			hasCode:    true,
		},
		{
			name:       "Passes through gRPC status",
			grpcStatus: serverErr,
			want:       serverErr,
			hasCode:    true,
		},
		{
			name:       "Passes through gRPC status when gRPC status also present",
			httpStatus: otherHTTPStatus,
			grpcStatus: serverErr,
			want:       serverErr,
			hasCode:    true,
		},
		{
			name:    "No statuses set",
			hasCode: false,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			err := Error{
				error:      errTest,
				httpStatus: tt.httpStatus,
				grpcStatus: tt.grpcStatus,
			}

			s, ok := err.GRPCStatus()

			require.Equal(t, tt.hasCode, ok)
			require.Equal(t, tt.want, s)
		})
	}
}