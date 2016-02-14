// The MIT License (MIT)

// Copyright (c) 2016 Claudemiro

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package main

import (
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"errors"
	"log"
	"os"

	_ "github.com/mattn/go-sqlite3"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/kms"
)

var (
	creds     *credentials.Credentials
	awsConfig *aws.Config
	svc       *kms.KMS
	keyID     string
)

func init() {
	creds = credentials.NewEnvCredentials()
	awsConfig = aws.NewConfig().WithRegion("us-east-1").WithCredentials(creds)
	svc = kms.New(session.New(), awsConfig)

	keyID = os.Getenv("AWS_KMS_KEY_ID")

	if keyID == "" {
		log.Fatal("You have to pass AWS_KMS_KEY_ID as a env var")
	}
}

// EncryptedString the type that works transparently with KMS
type EncryptedString string

func (e EncryptedString) Value() (driver.Value, error) {
	crypted, err := encrypt([]byte(e), svc, keyID)

	if err != nil {
		return nil, err
	}

	return driver.Value(base64.StdEncoding.EncodeToString(crypted)), nil
}

func (e *EncryptedString) Scan(src interface{}) error {
	var source string

	switch src.(type) {
	case string:
		source = src.(string)
	case []byte:
		source = string(src.([]byte))
	default:
		return errors.New("Incompatible type for EncryptedString")
	}

	decoded, err := base64.StdEncoding.DecodeString(source)

	if err != nil {
		return err
	}

	decrypted, err := decrypt(decoded, svc)

	if err != nil {
		return err
	}

	*e = EncryptedString(decrypted)

	return nil
}

func main() {
	db, err := sql.Open("sqlite3", "file:foo.db")

	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS users (
						id INTEGER PRIMARY KEY AUTOINCREMENT,
						email VARCHAR(50)
					  )`)

	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec("DELETE FROM users")

	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec("INSERT INTO users (email) VALUES(?)", EncryptedString("user@example.com"))

	if err != nil {
		log.Fatal(err)
	}

	rows, err := db.Query("SELECT * FROM users")

	if err != nil {
		log.Fatal(err)
	}

	defer rows.Close()

	for rows.Next() {
		var id int64
		var email EncryptedString

		err = rows.Scan(&id, &email)

		if err != nil {
			log.Println(err)
		}

		log.Println(email)
	}
}

// Encrypt returns a KMS encrypted byte array
func encrypt(payload []byte, svc *kms.KMS, keyID string) ([]byte, error) {
	params := &kms.EncryptInput{
		KeyId:     aws.String(keyID),
		Plaintext: payload,
		EncryptionContext: map[string]*string{
			"Key": aws.String("EncryptionContextValue"),
		},
		GrantTokens: []*string{
			aws.String("GrantTokenType"),
		},
	}
	resp, err := svc.Encrypt(params)

	if err != nil {
		return nil, err
	}

	return resp.CiphertextBlob, nil
}

// Decrypt returns a KMS decrypted byte array
func decrypt(payload []byte, svc *kms.KMS) ([]byte, error) {
	params := &kms.DecryptInput{
		CiphertextBlob: payload,
		EncryptionContext: map[string]*string{
			"Key": aws.String("EncryptionContextValue"),
		},
		GrantTokens: []*string{
			aws.String("GrantTokenType"),
		},
	}
	resp, err := svc.Decrypt(params)

	if err != nil {
		return nil, err
	}

	return resp.Plaintext, nil
}