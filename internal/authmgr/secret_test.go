package authmgr

import (
	"encoding/json"
	"io/fs"
	"reflect"
	"testing"

	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/amadeusitgroup/cds/internal/cos"
)

func createSecreFile(bom any, pathToFile string) error {
	data, err := json.Marshal(bom)
	if err != nil {
		return err
	}

	err = cos.WriteFile(pathToFile, data, fs.FileMode(0600))
	if err != nil {
		return err
	}

	return nil
}

func removeSecretFile(pathToFile string) error {
	err := cos.Fs.Remove(pathToFile)
	return err
}

func createFile(pathToFile string) error {
	_, err := cos.Fs.Create(pathToFile)
	return err
}

func removeFile(pathToFile string) error {
	err := cos.Fs.Remove(pathToFile)
	return err
}

func setupTest(t *testing.T, bom any) (teardown func()) {
	t.Helper()
	cos.SetMockedFileSystem()
	if bom == nil {
		if err := createFile(cenv.ConfigFile("secret.json")); err != nil {
			t.Fatal(err)
		}
		return func() {
			if err := removeFile(cenv.ConfigFile("secret.json")); err != nil {
				t.Fatal(err)
			}
			cos.SetRealFileSystem()
		}
	}
	if err := createSecreFile(bom, cenv.ConfigFile("secret.json")); err != nil {
		t.Fatal(err)
	}
	return func() {
		resetContent()
		if err := removeSecretFile(cenv.ConfigFile("secret.json")); err != nil {
			t.Fatal(err)
		}
		cos.SetRealFileSystem()
	}
}

func Test_repository_unmarshall(t *testing.T) {
	type fields struct {
		//Secrets map[string]Secret
		Repository repository
	}
	type args struct {
		path string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *repository
		wantErr bool
	}{
		{
			name: "secret.json",
			fields: fields{
				Repository: repository{
					Secrets: map[string]Secret{"key": {Login: "login", Metadata: []byte("metadata"), Raw: []byte("raw")}},
				},
			},
			args:    args{path: cenv.ConfigFile("secret.json")},
			wantErr: false,
			want: &repository{
				Secrets: map[string]Secret{"key": {Login: "login", Metadata: []byte("metadata"), Raw: []byte("raw")}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.fields.Repository)
			defer tearDown()
			got := &repository{}
			if err := got.unmarshall(tt.args.path); (err != nil) != tt.wantErr {
				t.Errorf("repository.unmarshall() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got = %v, want %v", got.Secrets, tt.want.Secrets)
			}
		})
	}
}

func Test_secretDetails(t *testing.T) {
	type args struct {
		key string
	}
	tests := []struct {
		name string
		bom  repository
		args args
		want Secret
	}{
		{
			name: "ok",
			bom:  repository{Secrets: map[string]Secret{"ar-tkn-rnd": {Login: "john", Metadata: []byte("toto: tata"), Raw: []byte("xGHBSHGSHH")}}},
			args: args{key: "ar-tkn-rnd"},
			want: Secret{Login: "john", Metadata: []byte("toto: tata"), Raw: []byte("xGHBSHGSHH")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.bom)
			defer tearDown()
			if got := secretDetails(tt.args.key); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_secretLogin(t *testing.T) {
	type args struct {
		key string
	}
	tests := []struct {
		name string
		bom  repository
		args args
		want string
	}{
		{
			name: "ok",
			bom:  repository{Secrets: map[string]Secret{"ar-tkn-rnd": {Login: "john", Metadata: []byte("toto: tata"), Raw: []byte("xGHBSHGSHH")}}},
			args: args{key: "ar-tkn-rnd"},
			want: "john",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.bom)
			defer tearDown()
			if got := secretLogin(tt.args.key); got != tt.want {
				t.Errorf("secretLogin() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_secretMetadata(t *testing.T) {
	type args struct {
		key string
	}
	tests := []struct {
		name string
		bom  repository
		args args
		want []byte
	}{
		{
			name: "ok",
			bom:  repository{Secrets: map[string]Secret{"ar-tkn-rnd": {Login: "john", Metadata: []byte("toto: tata"), Raw: []byte("xGHBSHGSHH")}}},
			args: args{key: "ar-tkn-rnd"},
			want: []byte("toto: tata"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.bom)
			defer tearDown()
			if got := secretMetadata(tt.args.key); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("secretMetadata() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_secretRaw(t *testing.T) {
	type args struct {
		key string
	}
	tests := []struct {
		name string
		bom  repository
		args args
		want []byte
	}{
		{
			name: "ok",
			bom:  repository{Secrets: map[string]Secret{"ar-tkn-rnd": {Login: "john", Metadata: []byte("toto: tata"), Raw: []byte("xGHBSHGSHH")}}},
			args: args{key: "ar-tkn-rnd"},
			want: []byte("xGHBSHGSHH"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.bom)
			defer tearDown()
			if got := secretRaw(tt.args.key); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("secretRaw() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_setLogin(t *testing.T) {
	type args struct {
		key string
		l   string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name:    "ok",
			args:    args{key: "ar-tkn-rnd", l: "john"},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, nil)
			defer tearDown()
			if err := setLogin(tt.args.key, tt.args.l); (err != nil) != tt.wantErr {
				t.Errorf("setLogin() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_setMetadata(t *testing.T) {
	type args struct {
		key string
		m   []byte
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name:    "ok",
			args:    args{key: "ar-tkn-rnd", m: []byte("toto: tata")},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, nil)
			defer tearDown()
			if err := setMetadata(tt.args.key, tt.args.m); (err != nil) != tt.wantErr {
				t.Errorf("setMetadata() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_setRaw(t *testing.T) {
	type args struct {
		key string
		r   []byte
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name:    "ok",
			args:    args{key: "ar-tkn-rnd", r: []byte("xGHBSHGSHH")},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, nil)
			defer tearDown()
			if err := setRaw(tt.args.key, tt.args.r); (err != nil) != tt.wantErr {
				t.Errorf("setRaw() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
