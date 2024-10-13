package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hairutdin/alexios/internal/logger"
	"github.com/hairutdin/alexios/internal/models"
	"github.com/hairutdin/alexios/internal/store"
	"go.uber.org/zap"
)

type app struct {
	store store.Store
}

func newApp(s store.Store) *app {
	return &app{store: s}
}

func parseSendCommand(command string) (string, string) {
	// Example of a command: "Send John Hello, how are you?"
	// Split the command into parts
	parts := strings.SplitN(command, " ", 3)

	// Ensure the command is well-formed
	if len(parts) < 3 {
		return "", ""
	}

	// The second part should be the recipient's username
	username := parts[1]

	// The third part should be the message text
	message := parts[2]

	return username, message
}

func parseReadCommand(command string) int {
	// Example of a command: "Read 1"
	// Split the command into parts
	parts := strings.Split(command, " ")

	// Ensure the command is well-formed
	if len(parts) < 2 {
		return -1 // Return an invalid index if the command is incorrect
	}

	// The second part should be the message index
	index, err := strconv.Atoi(parts[1])
	if err != nil || index < 1 {
		return -1 // Return an invalid index if the conversion fails or index is less than 1
	}

	return index - 1 // Convert to zero-based index
}

func parseRegisterCommand(command string) string {
	// Example of a command: "Sign Up JohnDoe"
	// Split the command into parts
	parts := strings.SplitN(command, " ", 3)

	// Ensure the command is well-formed
	if len(parts) < 3 {
		return ""
	}

	// The third part should be the username
	username := parts[2]

	return username
}

func (a *app) webhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if r.Method != http.MethodPost {
		logger.Log.Debug("got request with bad method", zap.String("method", r.Method))
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	logger.Log.Debug("decoding request")
	var req models.Request
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		logger.Log.Debug("cannot decode request JSON body", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if req.Request.Type != models.TypeSimpleUtterance {
		logger.Log.Debug("unsupported request type", zap.String("type", req.Request.Type))
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}

	// skill response text
	var text string

	switch true {
	// user asked to send a message
	case strings.HasPrefix(req.Request.Command, "Send"):
		// the hypothetical function parseSendCommand extracts
		// the recipient's login and the message text from the request
		username, message := parseSendCommand(req.Request.Command)

		// find the internal identifier of the addressee by his login name
		recepientID, err := a.store.FindRecepient(ctx, username)
		if err != nil {
			logger.Log.Debug("cannot find recepient by username", zap.String("username", username), zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// save the new message in DBMS, after successful saving it will become available for listening by the recipient
		err = a.store.SaveMessage(ctx, recepientID, store.Message{
			Sender:  req.Session.User.UserID,
			Time:    time.Now(),
			Payload: message,
		})
		if err != nil {
			logger.Log.Debug("cannot save message", zap.String("recepient", recepientID), zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Notify sender of the success of the operation
		text = "The message was sent successfully."

	// user asked to read a message
	case strings.HasPrefix(req.Request.Command, "Read"):
		// the hypothetical function parseReadCommand extracts from the request
		// the sequence number of the message in the list of available messages.
		messageIndex := parseReadCommand(req.Request.Command)

		// get the list of unheard messages of the user
		messages, err := a.store.ListMessages(ctx, req.Session.User.UserID)
		if err != nil {
			logger.Log.Debug("cannot load messages for user", zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		text = "There is no new messages for you."
		if len(messages) < messageIndex {
			// the user has asked to read a message that does not exist
			text = "There is no such a message."
		} else {

			// get the message by identifier
			messageID := messages[messageIndex].ID
			message, err := a.store.GetMessage(ctx, messageID)
			if err != nil {
				logger.Log.Debug("cannot load message", zap.Int64("id", messageID), zap.Error(err))
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			// pass the text of the message in the reply
			text = fmt.Sprintf("Message from %s, was sent at %s: %s", message.Sender, message.Time, message.Payload)
		}
	//	the user wants to register
	case strings.HasPrefix(req.Request.Command, "Sign Up"):
		// the hypothetical function parseRegisterCommand extracts
		// from the request the desired name of the new user
		username := parseRegisterCommand(req.Request.Command)

		// register a user
		err := a.store.RegisterUser(ctx, req.Session.User.UserID, username)
		// presence of a nonspecific error
		if err != nil && !errors.Is(err, store.ErrConflict) {
			logger.Log.Debug("cannot register user", zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		text = fmt.Sprintf("You have successfully been registered as %s", username)
		if errors.Is(err, store.ErrConflict) {
			text = "Sorry, this name has already been used. Try another name."
		}

	// if the command is not understood, just tell the user how many new messages a user has
	default:
		messages, err := a.store.ListMessages(ctx, req.Session.User.UserID)
		if err != nil {
			logger.Log.Debug("cannot load messages for user", zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		text = "There is no new messages for you."
		if len(messages) > 0 {
			text = fmt.Sprintf("There are %d new messages.", len(messages))
		}

		// the first request of a new session
		if req.Session.New {
			tz, err := time.LoadLocation(req.Timezone)
			if err != nil {
				logger.Log.Debug("cannot parse timezone")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			now := time.Now().In(tz)
			hour, minute, _ := now.Clock()

			text = fmt.Sprintf("Exact time %d hours, %d minutes. %s", hour, minute, text)
		}
	}

	resp := models.Response{
		Response: models.ResponsePayload{
			Text: text, // Alexios will pronounce the text
		},
		Version: "1.0",
	}

	w.Header().Set("Content-Type", "application/json")

	enc := json.NewEncoder(w)
	if err := enc.Encode(resp); err != nil {
		logger.Log.Debug("error encoding response", zap.Error(err))
		return
	}
	logger.Log.Debug("sending HTTP 200 response")
}
