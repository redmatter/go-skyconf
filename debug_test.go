package skyconf

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestString(t *testing.T) {
	type args struct {
		cfg          interface{}
		withUntagged bool
		formatters   []Formatter
	}
	tests := []struct {
		name    string
		args    args
		wantStr string
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "no formatters",
			args: args{
				cfg:          struct{}{},
				withUntagged: false,
				formatters:   nil,
			},
			wantStr: "",
			wantErr: assert.Error,
		},
		{
			name: "single formatter",
			args: args{
				cfg: &struct {
					Level string
					DB    struct {
						Host     string `sky:",default:localhost,source:region"`
						Port     int    `sky:",default:5432,optional"`
						Password string `sky:",source:global"`
					} `sky:"db"`
				}{},
				withUntagged: false,
				formatters: []Formatter{
					SSMSourceWithID(nil, "/path/global", "global"),
					SSMSourceWithID(nil, "/path/region1", "region"),
				},
			},
			wantStr: "region:/path/region1/db/host: {defaultValue:localhost optional:false flatten:false source:region}\n" +
				"anyOf:[global:/path/global/db/port,region:/path/region1/db/port]: {defaultValue:5432 optional:true flatten:false source:}\n" +
				"global:/path/global/db/password: {defaultValue: optional:false flatten:false source:global}",
			wantErr: assert.NoError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStr, err := String(tt.args.cfg, tt.args.withUntagged, tt.args.formatters...)
			if !tt.wantErr(t, err) {
				return
			}

			assert.Equal(t, tt.wantStr, gotStr)
		})
	}
}
