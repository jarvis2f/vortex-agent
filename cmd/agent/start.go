package agent

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/jarvis2f/vortex-agent/agent"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

var startCmd = &cobra.Command{
	Use:  "start",
	Long: `start agent service`,
	Run: func(cmd *cobra.Command, args []string) {
		id := cmd.Flag("id").Value.String()
		server := cmd.Flag("server").Value.String()
		key := cmd.Flag("key").Value.String()
		if agent.Config == "" && (id == "" || server == "" || key == "") {
			fmt.Println("start failed, must provide config file or id, server, key")
			os.Exit(1)
		}
		if agent.Config != "" {
			id, server, key = loadConfig()
		}
		start(id, server, key)
	},
}

func init() {
	agentCmd.AddCommand(startCmd)

	startCmd.Flags().StringP("id", "i", "", "agent id")
	startCmd.Flags().StringP("server", "s", "", "server address")
	startCmd.Flags().StringP("key", "k", "", "server key")
}

type InstallBody struct {
	Id        string `json:"id"`
	Key       string `json:"key"`
	Signature string `json:"signature"`
}

type InstallResponse struct {
	Link string `json:"link"`
}

func start(id string, server string, serverKey string) {
	if agent.Log == nil || agent.LogR == nil {
		agent.InitLog("info", "remote")
	}
	agent.Log.Sugar().Debugf("start agent with id: %s, server: %s, key: %s", id, server, serverKey)
	url := server + "/api/v1/agent/install"
	agentPrivateKey, _ := ecdh.P256().GenerateKey(rand.Reader)
	agentPublicKey := agentPrivateKey.PublicKey()
	serverKeyBytes, err := hex.DecodeString(serverKey)
	if err != nil {
		agent.Log.Fatal("start failed", zap.Error(err))
		return
	}
	serverPublicKey, err := ecdh.P256().NewPublicKey(serverKeyBytes)
	if err != nil {
		agent.Log.Fatal("start failed", zap.Error(err))
		return
	}
	sharedSecret, _ := agentPrivateKey.ECDH(serverPublicKey)
	secret := hex.EncodeToString(sharedSecret)

	body := InstallBody{
		Id:  id,
		Key: fmt.Sprintf("%x", agentPublicKey.Bytes()),
		Signature: agent.Sign(map[string]interface{}{
			"id":  id,
			"key": fmt.Sprintf("%x", agentPublicKey.Bytes()),
		}, secret),
	}
	bodyBytes, _ := json.Marshal(body)

	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		return
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(bodyBytes))
	if err != nil {
		agent.Log.Fatal("start failed", zap.Error(err))
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode != 200 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		agent.Log.Sugar().Fatalf("start failed status: %s body: \n%s", resp.Status, string(bodyBytes))
	}

	var installResponse InstallResponse
	if err = json.NewDecoder(resp.Body).Decode(&installResponse); err != nil {
		agent.Log.Fatal("start failed, decode response fail.", zap.Error(err))
	}

	decrypt, err := agent.Decrypt(installResponse.Link, []byte(secret))
	if err != nil {
		agent.Log.Fatal("start failed, decrypt link fail.", zap.Error(err))
	}

	var agentOption agent.Options
	if err = json.Unmarshal(decrypt, &agentOption); err != nil {
		agent.Log.Fatal("start failed, decode option fail.", zap.Error(err))
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	agent.GlobalAgent = agent.NewAgent(agentOption, id)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go agent.GlobalAgent.Start(ctx)

	sig := <-sigCh
	agent.Log.Sugar().Infof("receive signal: %s", sig.String())
	cancel()

	<-ctx.Done()
	agent.GlobalAgent.Stop()
}

func loadConfig() (string, string, string) {
	var config map[string]string
	file, err := os.Open(agent.Config)
	if err != nil {
		fmt.Println("open config file fail", err)
		os.Exit(1)
	}

	defer func(file *os.File) {
		_ = file.Close()
	}(file)

	if err := json.NewDecoder(file).Decode(&config); err != nil {
		fmt.Println("decode config file fail", err)
		os.Exit(1)
	}

	if config["logLevel"] != "" && config["logDest"] != "" {
		agent.InitLog(config["logLevel"], config["logDest"])
	}
	if config["dir"] != "" {
		agent.Dir = config["dir"]
	}

	return config["id"], config["server"], config["key"]
}
