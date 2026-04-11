package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// resolveAgentUUID looks up the agent UUID from the channel's agent key.
// Returns uuid.Nil if the agent key is empty or not found.
func (c *Channel) resolveAgentUUID(ctx context.Context) (uuid.UUID, error) {
	key := c.AgentID()
	if key == "" {
		return uuid.Nil, fmt.Errorf("no agent key configured")
	}

	// Try direct UUID parse first (future-proofing).
	if id, err := uuid.Parse(key); err == nil {
		return id, nil
	}

	// Inject tenant scope so the store can filter by tenant_id.
	ctx = store.WithTenantID(ctx, c.TenantID())
	agent, err := c.agentStore.GetByKey(ctx, key)
	if err != nil {
		return uuid.Nil, fmt.Errorf("agent %q not found: %w", key, err)
	}
	return agent.ID, nil
}

// handleBotCommand checks if the message is a known bot command and handles it.
// Returns true if the message was handled as a command.
func (c *Channel) handleBotCommand(ctx context.Context, message *telego.Message, chatID int64, chatIDStr, localKey, text, senderID string, isGroup, isForum bool, messageThreadID int) bool {
	if len(text) == 0 || text[0] != '/' {
		return false
	}

	// Extract command (strip @botname suffix if present)
	cmd := strings.SplitN(text, " ", 2)[0]
	cmd = strings.ToLower(cmd)

	// In groups, ignore commands addressed to other bots (e.g. /help@other_bot)
	if isGroup {
		if parts := strings.SplitN(cmd, "@", 2); len(parts) == 2 {
			if !strings.EqualFold(parts[1], c.bot.Username()) {
				return false
			}
		}
	}

	cmd = strings.SplitN(cmd, "@", 2)[0]

	// Inject tenant scope so all command handlers have tenant_id in context.
	ctx = store.WithTenantID(ctx, c.TenantID())

	chatIDObj := tu.ID(chatID)

	// Helper: set MessageThreadID on outgoing messages for forum topics.
	// TS ref: buildTelegramThreadParams() — General topic (1) must be omitted.
	setThread := func(msg *telego.SendMessageParams) {
		sendThreadID := resolveThreadIDForSend(messageThreadID)
		if sendThreadID > 0 {
			msg.MessageThreadID = sendThreadID
		}
	}

	switch cmd {
	case "/start":
		// Don't intercept /start — let it pass through to agent loop.
		return false

	case "/help":
		helpText := "Available commands:\n" +
			"/start — Start chatting with the bot\n" +
			"/help — Show this help message\n" +
			"/stop — Stop current running task\n" +
			"/stopall — Stop all running tasks\n" +
			"/reset — Reset conversation history\n" +
			"/status — Show bot status\n" +
			"/pair — Pair this account with your org email\n" +
			"/reactions — Show reaction emoji legend\n" +
			"/tasks — List team tasks\n" +
			"/task_detail <id> — View task detail\n" +
			"/subagents — List subagent tasks\n" +
			"/subagent <id> — View subagent task detail\n" +
			"/writers — List file writers for this group\n" +
			"/addwriter — Add a file writer (reply to their message)\n" +
			"/removewriter — Remove a file writer (reply to their message)\n" +
			"\nJust send a message to chat with the AI."
		msg := tu.Message(chatIDObj, helpText)
		setThread(msg)
		c.bot.SendMessage(ctx, msg)
		return true

	case "/reset":
		// In group chats, only file writers can reset conversation history.
		if isGroup && c.configPermStore != nil {
			agentID, err := c.resolveAgentUUID(ctx)
			if err == nil {
				groupID := fmt.Sprintf("group:%s:%s", c.Name(), chatIDStr)
				senderNumericID := strings.SplitN(senderID, "|", 2)[0]
				isWriter, err := c.configPermStore.CheckPermission(ctx, agentID, groupID, store.ConfigTypeFileWriter, senderNumericID)
				if err != nil {
					slog.Warn("security.reset_writer_check_failed", "error", err, "sender", senderNumericID)
					// fail-open: allow reset if DB check fails
				} else if !isWriter {
					msg := tu.Message(chatIDObj, "Only file writers can reset conversation history in this group.")
					setThread(msg)
					c.bot.SendMessage(ctx, msg)
					return true
				}
			}
		}

		// Fix: use correct PeerKind so the gateway consumer builds the right session key.
		peerKind := "direct"
		if isGroup {
			peerKind = "group"
		}
		c.Bus().PublishInbound(bus.InboundMessage{
			Channel:  c.Name(),
			SenderID: senderID,
			ChatID:   chatIDStr,
			Content:  "/reset",
			PeerKind: peerKind,
			AgentID:  c.AgentID(),
			UserID:   strings.SplitN(senderID, "|", 2)[0],
			TenantID: c.TenantID(),
			Metadata: map[string]string{
				"command":           "reset",
				"local_key":         localKey,
				"is_forum":          fmt.Sprintf("%t", isForum),
				"message_thread_id": fmt.Sprintf("%d", messageThreadID),
			},
		})
		msg := tu.Message(chatIDObj, "Conversation history has been reset.")
		setThread(msg)
		c.bot.SendMessage(ctx, msg)
		return true

	case "/stop":
		peerKind := "direct"
		if isGroup {
			peerKind = "group"
		}
		c.Bus().PublishInbound(bus.InboundMessage{
			Channel:  c.Name(),
			SenderID: senderID,
			ChatID:   chatIDStr,
			Content:  "/stop",
			PeerKind: peerKind,
			AgentID:  c.AgentID(),
			UserID:   strings.SplitN(senderID, "|", 2)[0],
			TenantID: c.TenantID(),
			Metadata: map[string]string{
				"command":           "stop",
				"local_key":         localKey,
				"is_forum":          fmt.Sprintf("%t", isForum),
				"message_thread_id": fmt.Sprintf("%d", messageThreadID),
			},
		})
		// Feedback is sent by the consumer after cancel result is known.
		return true

	case "/stopall":
		peerKind := "direct"
		if isGroup {
			peerKind = "group"
		}
		c.Bus().PublishInbound(bus.InboundMessage{
			Channel:  c.Name(),
			SenderID: senderID,
			ChatID:   chatIDStr,
			Content:  "/stopall",
			PeerKind: peerKind,
			AgentID:  c.AgentID(),
			UserID:   strings.SplitN(senderID, "|", 2)[0],
			TenantID: c.TenantID(),
			Metadata: map[string]string{
				"command":           "stopall",
				"local_key":         localKey,
				"is_forum":          fmt.Sprintf("%t", isForum),
				"message_thread_id": fmt.Sprintf("%d", messageThreadID),
			},
		})
		// Feedback is sent by the consumer after cancel result is known.
		return true

	case "/status":
		statusText := fmt.Sprintf("Bot status: Running\nChannel: Telegram\nBot: @%s", c.bot.Username())
		msg := tu.Message(chatIDObj, statusText)
		setThread(msg)
		c.bot.SendMessage(ctx, msg)
		return true

	case "/tasks":
		c.handleTasksList(ctx, chatID, isGroup, setThread)
		return true

	case "/task_detail":
		c.handleTaskDetail(ctx, chatID, text, isGroup, setThread)
		return true

	case "/subagents":
		c.handleSubagentsList(ctx, chatID, isGroup, setThread)
		return true

	case "/subagent":
		c.handleSubagentDetail(ctx, chatID, text, isGroup, setThread)
		return true

	case "/addwriter":
		c.handleWriterCommand(ctx, message, chatID, chatIDStr, senderID, isGroup, setThread, "add")
		return true

	case "/removewriter":
		c.handleWriterCommand(ctx, message, chatID, chatIDStr, senderID, isGroup, setThread, "remove")
		return true

	case "/writers":
		c.handleListWriters(ctx, chatID, chatIDStr, isGroup, setThread)
		return true

	case "/reactions":
		var lines strings.Builder
		for _, r := range reactionLegend {
			lines.WriteString(fmt.Sprintf("%s  %s\n", r.Emoji, r.Desc))
		}
		reactText := fmt.Sprintf("<b>Reaction Emoji Legend</b>\n\n<pre>%s</pre>\nReaction level: <b>%s</b>", lines.String(), c.config.ReactionLevel)
		msg := tu.Message(chatIDObj, reactText)
		msg.ParseMode = telego.ModeHTML
		setThread(msg)
		c.bot.SendMessage(ctx, msg)
		return true

	case "/pair":
		c.handlePairCommand(ctx, chatID, senderID, chatIDStr, setThread)
		return true

	case "/unpair":
		c.clearPairingFlow(senderID)
		msg := tu.Message(chatIDObj, "Pairing flow cancelled.")
		setThread(msg)
		c.bot.SendMessage(ctx, msg)
		return true
	}

	return false
}

// handlePairingFlowInput intercepts messages from users in an active pairing flow.
// Returns true if the message was consumed by the pairing flow.
func (c *Channel) handlePairingFlowInput(ctx context.Context, chatID int64, senderID, text string, setThread func(*telego.SendMessageParams)) bool {
	if c.pairingHandler == nil {
		return false
	}

	val, ok := c.pairingFlows.Load(senderID)
	if !ok {
		return false
	}
	flow := val.(*channels.PairingFlowEntry)

	// Expire stale flows (15 minutes).
	if time.Since(flow.StartedAt) > 15*time.Minute {
		c.pairingFlows.Delete(senderID)
		return false
	}

	chatIDObj := tu.ID(chatID)

	switch flow.State {
	case channels.PairingStateAwaitEmail:
		if !channels.IsPossibleEmail(text) {
			msg := tu.Message(chatIDObj, "That doesn't look like an email address. Please enter your organization email, or /unpair to cancel.")
			setThread(msg)
			c.bot.SendMessage(ctx, msg)
			return true
		}

		reply := c.pairingHandler.HandleEmailInput(ctx, flow.ChannelType, senderID, flow.ChatID, text)
		msg := tu.Message(chatIDObj, reply)
		setThread(msg)
		c.bot.SendMessage(ctx, msg)

		// If OTP was sent successfully, advance state to await code.
		if strings.Contains(reply, "verification code") {
			flow.State = channels.PairingStateAwaitCode
			c.pairingFlows.Store(senderID, flow)
		}
		return true

	case channels.PairingStateAwaitCode:
		if !channels.IsPossibleOTP(text) {
			msg := tu.Message(chatIDObj, "Please enter the 6-digit verification code, or /unpair to cancel.")
			setThread(msg)
			c.bot.SendMessage(ctx, msg)
			return true
		}

		reply, paired := c.pairingHandler.HandleCodeInput(ctx, flow.ChannelType, senderID, flow.ChatID, text)
		msg := tu.Message(chatIDObj, reply)
		setThread(msg)
		c.bot.SendMessage(ctx, msg)

		if paired {
			c.pairingFlows.Delete(senderID)
		} else if strings.Contains(reply, "start over") {
			c.pairingFlows.Delete(senderID)
		}
		return true
	}

	return false
}

// handlePairCommand initiates the /pair flow.
func (c *Channel) handlePairCommand(ctx context.Context, chatID int64, senderID, chatIDStr string, setThread func(*telego.SendMessageParams)) {
	chatIDObj := tu.ID(chatID)

	if c.pairingHandler == nil {
		msg := tu.Message(chatIDObj, "Email pairing is not configured. Contact your administrator.")
		setThread(msg)
		c.bot.SendMessage(ctx, msg)
		return
	}

	reply := c.pairingHandler.HandlePairCommand(ctx, c.Name(), senderID, chatIDStr)
	msg := tu.Message(chatIDObj, reply)
	setThread(msg)
	c.bot.SendMessage(ctx, msg)

	// If the user is not already paired, start the flow.
	if strings.Contains(reply, "email address") {
		c.pairingFlows.Store(senderID, &channels.PairingFlowEntry{
			State:       channels.PairingStateAwaitEmail,
			ChannelType: c.Name(),
			SenderID:    senderID,
			ChatID:      chatIDStr,
			StartedAt:   time.Now(),
		})
	}
}

// clearPairingFlow removes a pairing flow for a sender.
func (c *Channel) clearPairingFlow(senderID string) {
	c.pairingFlows.Delete(senderID)
}
