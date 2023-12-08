package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"

	"os"
	"time"

	"github.com/jlaffaye/ftp"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type Config struct {
	User          string `json:"User"`
	Password      string `json:"Password"`
	Server        string `json:"Server"`
	Port          int    `json:"Port"`
	Protocol      string `json:"Protocol"`
	IgnoreHostKey bool   `json:"IgnoreHostKey"`
	FilesToUpload []struct {
		LocalPath                  string `json:"localPath"`
		RemotePath                 string `json:"remotePath"`
		DeleteLocalFileAfterUpload bool   `json:"deleteLocalFileAfterUpload"`
	} `json:"filesToUpload"`
	FilesToDownload []struct {
		LocalPath                     string `json:"localPath"`
		RemotePath                    string `json:"remotePath"`
		DeleteRemoteFileAfterDownload bool   `json:"deleteRemoteFileAfterDownload"`
	} `json:"filesToDownload"`
}

// jsonFileReader reads for the json file we attached and then reads it with the os.ReadFile
func jsonFileReader(configFile string) (Config, error) {

	// f stores the data from the json file
	var f Config

	data, derr := os.ReadFile(configFile)

	if derr != nil {
		return f, derr
	}

	err := json.Unmarshal(data, &f)

	if err != nil {
		return f, err
	}

	return f, err

}

// selectProtocol determines what protocol should be used
func selectProtocol(f Config) error {

	switch f.Protocol {
	case "ftp":
		fmt.Println("In FTP mode")
		return ftpProtocol(f)

	case "sftp":
		fmt.Println("In SFTP mode")
		return sftpProtocol(f)

	default:

		return fmt.Errorf("Invalid protocol, protocol should be ftp or sftp")
	}
}

// func cancelAction(s Config) error {

// 	ctx := context.Background()
// 	ctx, cancel := context.WithCancel(ctx)

// 	go func

// }

func ftpProtocol(config Config) error {

	fmt.Println("Connecting to FTP server")

	conn, err := ftp.Dial(fmt.Sprintf("%s:%d", config.Server, config.Port),
		ftp.DialWithTimeout(5*time.Second),
		ftp.DialWithExplicitTLS(&tls.Config{
			ServerName:             config.Server,
			SessionTicketsDisabled: false,
			ClientSessionCache:     tls.NewLRUClientSessionCache(0),
		}),
	)

	if err != nil {
		return err
	}

	defer conn.Quit()

	err = conn.Login(config.User, config.Password)
	if err != nil {
		return err
	}

	//check for error
	for _, download := range config.FilesToDownload {

		resp, err := conn.Retr(download.RemotePath)

		if err != nil {
			return err
		}

		b, err := io.ReadAll(resp)
		if err != nil {
			return err
		}

		err = os.WriteFile(download.LocalPath, b, 0600)
		if err != nil {
			return err
		}

		if download.DeleteRemoteFileAfterDownload {
			err := conn.Delete(download.RemotePath)

			if err != nil {
				return err
			}
		}
	}

	// check for error
	for _, upload := range config.FilesToUpload {
		r, err := os.Open(upload.LocalPath)

		if err != nil {
			return err
		}

		err = conn.Stor(upload.RemotePath, r)

		if err != nil {
			return err
		}

		if upload.DeleteLocalFileAfterUpload {
			err = os.Remove(upload.LocalPath)

			if err != nil {
				return err
			}
		}
	}

	return nil
}

// sftpProtocol
func sftpProtocol(config Config) error {

	sshConfig := &ssh.ClientConfig{

		User: config.User,
		Auth: []ssh.AuthMethod{
			ssh.Password(config.Password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	conn, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", config.Server, config.Port), sshConfig)
	if err != nil {
		return err
	}
	defer conn.Close() // its equal to quit in the ftp protocol

	client, err := sftp.NewClient(conn)
	if err != nil {
		return err
	}
	defer client.Close()

	for _, download := range config.FilesToDownload {

		remotePath := download.RemotePath
		localPath := download.LocalPath

		remoteFile, err := client.Open(remotePath) // reads the file, in this case remotepath
		if err != nil {
			fmt.Printf("Error opening remote file : %v \n", err)
			return err
		}
		defer remoteFile.Close()

		localFile, err := os.Create(localPath)
		if err != nil {
			fmt.Printf("Error creating a local file: %v\n", err)
			return err
		}
		defer localFile.Close()

		_, err = io.Copy(localFile, remoteFile)
		if err != nil {
			fmt.Printf("Error copying data: %v\n", err)
			return err
		}

		if download.DeleteRemoteFileAfterDownload {
			err = client.Remove(remotePath)
			if err != nil {
				fmt.Printf("Error deleting remote file: %v\n", err)
				return err
			}
		}
	}

	for _, upload := range config.FilesToUpload {

		localPath := upload.LocalPath
		remotePath := upload.RemotePath

		localFile, err := os.Open(localPath)
		if err != nil {
			fmt.Printf("Error opening local file: %v \n", err)
			return err
		}
		defer localFile.Close()

		remoteFile, err := client.Create(remotePath)
		if err != nil {
			fmt.Printf("Error creating remote file: %v\n", err)
			return err
		}
		defer remoteFile.Close()

		_, err = io.Copy(remoteFile, localFile)
		if err != nil {
			fmt.Printf("Error copying data: %v\n", err)
			return err
		}

		if upload.DeleteLocalFileAfterUpload {
			err = os.Remove(localPath)
			if err != nil {
				fmt.Printf("Error deleting local file: %v\n", err)
				return err
			}

		}
	}

	return nil
}

func main() {
	fmt.Println("Reading config file")

	config, err := jsonFileReader("properties.json")

	if err != nil {
		log.Fatal("Error reading the JSON file: ", err)
	}

	fmt.Printf("Will connect to %s:%d with username %s\n", config.Server, config.Port, config.User)

	err = selectProtocol(config)

	if err != nil {
		log.Fatal("Error: ", err)
	}
}
