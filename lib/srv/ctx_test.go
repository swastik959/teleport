/*
Copyright 2022 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package srv

import (
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"github.com/gravitational/teleport"
	"github.com/gravitational/teleport/api/types"
	apievents "github.com/gravitational/teleport/api/types/events"
	"github.com/gravitational/teleport/lib/services"
)

func TestCheckSFTPAllowed(t *testing.T) {
	srv := newMockServer(t)
	ctx := newTestServerContext(t, srv, nil)

	tests := []struct {
		name                 string
		nodeAllowFileCopying bool
		roles                []types.Role
		expectedErr          error
	}{
		{
			name:                 "node disallowed",
			nodeAllowFileCopying: false,
			roles: []types.Role{
				&types.RoleV6{
					Kind: types.KindNode,
				},
			},
			expectedErr: ErrNodeFileCopyingNotPermitted,
		},
		{
			name:                 "node allowed",
			nodeAllowFileCopying: true,
			roles: []types.Role{
				&types.RoleV6{
					Kind: types.KindNode,
				},
			},
			expectedErr: nil,
		},
		{
			name:                 "role disallowed",
			nodeAllowFileCopying: true,
			roles: []types.Role{
				&types.RoleV6{
					Kind: types.KindNode,
					Spec: types.RoleSpecV6{
						Options: types.RoleOptions{
							SSHFileCopy: types.NewBoolOption(false),
						},
					},
				},
			},
			expectedErr: errRoleFileCopyingNotPermitted,
		},
		{
			name:                 "role allowed",
			nodeAllowFileCopying: true,
			roles: []types.Role{
				&types.RoleV6{
					Kind: types.KindNode,
					Spec: types.RoleSpecV6{
						Options: types.RoleOptions{
							SSHFileCopy: types.NewBoolOption(true),
						},
					},
				},
			},
			expectedErr: nil,
		},
		{
			name:                 "conflicting roles",
			nodeAllowFileCopying: true,
			roles: []types.Role{
				&types.RoleV6{
					Kind: types.KindNode,
					Spec: types.RoleSpecV6{
						Options: types.RoleOptions{
							SSHFileCopy: types.NewBoolOption(true),
						},
					},
				},
				&types.RoleV6{
					Kind: types.KindNode,
					Spec: types.RoleSpecV6{
						Options: types.RoleOptions{
							SSHFileCopy: types.NewBoolOption(false),
						},
					},
				},
			},
			expectedErr: errRoleFileCopyingNotPermitted,
		},
		{
			name:                 "moderated sessions enforced",
			nodeAllowFileCopying: true,
			roles: []types.Role{
				&types.RoleV6{
					Kind: types.KindNode,
					Spec: types.RoleSpecV6{
						Allow: types.RoleConditions{
							RequireSessionJoin: []*types.SessionRequirePolicy{
								{
									Name:   "test",
									Filter: `contains(user.roles, "auditor")`,
									Kinds:  []string{string(types.SSHSessionKind)},
									Modes:  []string{string(types.SessionModeratorMode)},
									Count:  3,
								},
							},
						},
					},
				},
			},
			expectedErr: errCannotStartUnattendedSession,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx.AllowFileCopying = tt.nodeAllowFileCopying

			roles := services.NewRoleSet(tt.roles...)

			ctx.Identity.AccessChecker = services.NewAccessCheckerWithRoleSet(
				&services.AccessInfo{
					Roles: roles.RoleNames(),
				},
				"localhost",
				roles,
			)

			err := ctx.CheckSFTPAllowed()
			if tt.expectedErr == nil {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tt.expectedErr.Error())
			}
		})
	}
}

func TestIdentityContext_GetUserMetadata(t *testing.T) {
	tests := []struct {
		name  string
		idCtx IdentityContext
		want  apievents.UserMetadata
	}{
		{
			name: "user metadata",
			idCtx: IdentityContext{
				TeleportUser:   "alpaca",
				Impersonator:   "llama",
				Login:          "alpaca1",
				ActiveRequests: []string{"access-req1", "access-req2"},
			},
			want: apievents.UserMetadata{
				User:           "alpaca",
				Login:          "alpaca1",
				Impersonator:   "llama",
				AccessRequests: []string{"access-req1", "access-req2"},
			},
		},
		{
			name: "device metadata",
			idCtx: IdentityContext{
				TeleportUser: "alpaca",
				Login:        "alpaca1",
				Certificate: &ssh.Certificate{
					Permissions: ssh.Permissions{
						Extensions: map[string]string{
							teleport.CertExtensionDeviceID:           "deviceid1",
							teleport.CertExtensionDeviceAssetTag:     "assettag1",
							teleport.CertExtensionDeviceCredentialID: "credentialid1",
						},
					},
				},
			},
			want: apievents.UserMetadata{
				User:  "alpaca",
				Login: "alpaca1",
				TrustedDevice: &apievents.DeviceMetadata{
					DeviceId:     "deviceid1",
					AssetTag:     "assettag1",
					CredentialId: "credentialid1",
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.idCtx.GetUserMetadata()
			want := test.want
			if !proto.Equal(&got, &want) {
				t.Errorf("GetUserMetadata mismatch (-want +got):\n%s", cmp.Diff(want, got))
			}
		})
	}
}
