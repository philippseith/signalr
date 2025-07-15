package ext

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/mojtabaRKS/signalr"
	"github.com/redis/go-redis/v9"
)

const (
	redisChannelPrefix      = "signalr:"
	redisAllChannel         = redisChannelPrefix + "all"    // Channel for InvokeAll and InvokeClient messages
	redisGroupChannelPrefix = redisChannelPrefix + "group:" // Channel for InvokeGroup messages
	// redisClientChannelPrefix   = redisChannelPrefix + "client:" // Not used with current invoke approach
	redisGroupMembershipPrefix = redisChannelPrefix + "groupmembers:" // Key prefix for Redis Sets storing group members (Set key: signalr:groupmembers:<groupName>, Value: {connectionID1, connectionID2, ...})
	// redisConnectionGroupsPrefix = redisChannelPrefix + "connectiongroups:" // Not strictly needed with Redis Sets for membership; local cache is sufficient.
)

// redisInvocation represents a message sent over Redis for hub invocations
type redisInvocation struct {
	Target       string        `json:"t"`
	Args         []interface{} `json:"a"`
	ExcludedIDs  []string      `json:"e,omitempty"` // Optional: Connection IDs to exclude (e.g., for SendAllExcept)
	GroupName    string        `json:"g,omitempty"` // Used for group messages to identify the target group
	ConnectionID string        `json:"c,omitempty"` // Used for client messages to identify the target client
}

// redisHubLifetimeManager manages hub connections using Redis Pub/Sub and Sets
type redisHubLifetimeManager struct {
	clients          sync.Map // Map[string]signalr.HubConnection - Local connections on this server instance
	connectionGroups sync.Map // Map[string]map[string]struct{} - Local cache: connectionID -> set of groupNames this connection belongs to *locally*
	redisClient      *redis.Client
	pubSub           *redis.PubSub
	logger           signalr.StructuredLogger
	instanceID       string // Unique ID for this server instance (can be useful for debugging)
	ctx              context.Context
	cancel           context.CancelFunc
	// protocol         signalr.hubProtocol // Needed for serialization if args are complex (though JSON handles basic types)
}

// newRedisHubLifetimeManager creates a new Redis-backed HubLifetimeManager
func newRedisHubLifetimeManager(ctx context.Context, logger signalr.StructuredLogger, redisclient *redis.Client) (signalr.HubLifetimeManager, error) {
	instanceID := uuid.New().String()
	info := log.WithPrefix(logger, "ts", log.DefaultTimestampUTC, "class", "redisHubLifetimeManager", "instance", instanceID)

	mgrCtx, cancel := context.WithCancel(ctx)

	manager := &redisHubLifetimeManager{
		redisClient: redisclient,
		logger:      info,
		instanceID:  instanceID,
		ctx:         mgrCtx,
		cancel:      cancel,
		// protocol:    protocol, // Store protocol (though json.Marshal handles args for now)
	}

	// Start listening to Redis Pub/Sub in a separate goroutine
	go manager.subscribeLoop()

	level.Info(manager.logger).Log("event", "Redis HubLifetimeManager started")
	return manager, nil
}

// subscribeLoop listens to relevant Redis channels
func (r *redisHubLifetimeManager) subscribeLoop() {
	// Subscribe to the 'all' channel and all 'group' channels
	// PSubscribe allows pattern matching
	allPattern := redisAllChannel
	groupPattern := r.groupChannel("*")
	r.pubSub = r.redisClient.PSubscribe(r.ctx, allPattern, groupPattern)

	// Wait for confirmation that subscription is created before proceeding.
	_, err := r.pubSub.Receive(r.ctx)
	if err != nil {
		level.Error(r.logger).Log("event", "redis_subscribe_failed", "err", err, "patterns", []string{allPattern, groupPattern})
		r.cancel() // Signal manager is defunct
		return
	}

	err = level.Info(r.logger).Log("event", "redis_subscribed", "patterns", []string{allPattern, groupPattern})
	if err != nil {
		return
	}

	ch := r.pubSub.Channel()

	for {
		select {
		case <-r.ctx.Done():
			level.Info(r.logger).Log("event", "redis_subscribe_loop_stopping")
			_ = r.pubSub.Close() // Ensure PubSub is closed when context is cancelled
			return
		case msg, ok := <-ch:
			if !ok {
				level.Info(r.logger).Log("event", "redis_pubsub_channel_closed")
				// Attempt to resubscribe or handle error? For now, just return.
				// Consider adding retry logic here.
				r.cancel() // Signal manager is defunct if channel closes unexpectedly
				return
			}
			// Check if the message matches one of the subscribed patterns
			// (PSubscribe sends messages with Pattern and Channel fields)
			if msg.Channel == allPattern || strings.HasPrefix(msg.Channel, redisGroupChannelPrefix) {
				r.handleRedisMessage(msg)
			} else {
				level.Debug(r.logger).Log("event", "received_redis_message_unmatched_pattern", "channel", msg.Channel, "pattern", msg.Pattern, "payload", msg.Payload)
			}
		}
	}
}

// handleRedisMessage processes messages received from Redis Pub/Sub
func (r *redisHubLifetimeManager) handleRedisMessage(msg *redis.Message) {
	level.Debug(r.logger).Log("event", "received_redis_message", "channel", msg.Channel, "payload_len", len(msg.Payload))

	var invocation redisInvocation
	if err := json.Unmarshal([]byte(msg.Payload), &invocation); err != nil {
		level.Error(r.logger).Log("event", "redis_payload_unmarshal_error", "err", err, "payload", msg.Payload)
		return
	}

	// Create exclusion map for faster lookup
	excludedMap := make(map[string]struct{}, len(invocation.ExcludedIDs))
	for _, id := range invocation.ExcludedIDs {
		excludedMap[id] = struct{}{}
	}

	// --- Message Dispatching Logic ---

	// 1. Message for a specific client (sent via InvokeClient)
	if invocation.ConnectionID != "" {
		// Check if this specific client is managed by *this* server instance
		if conn, ok := r.clients.Load(invocation.ConnectionID); ok {
			// Check if this connection ID is in the exclusion list
			if _, excluded := excludedMap[invocation.ConnectionID]; !excluded {
				hubConn := conn.(signalr.HubConnection)
				// Send the invocation asynchronously
				go func() {
					// Use empty invocation ID as per CHANGE pattern interpretation
					// Timeout removed as per request
					err := hubConn.SendInvocation("", invocation.Target, invocation.Args)
					if err != nil {
						level.Error(r.logger).Log("event", "send_invocation_failed", "target", "client", "connectionId", invocation.ConnectionID, "err", err)
						// Consider removing connection if send fails repeatedly?
					} else {
						level.Debug(r.logger).Log("event", "sent_invocation_local", "target", "client", "connectionId", invocation.ConnectionID, "method", invocation.Target)
					}
				}()
			} else {
				level.Debug(r.logger).Log("event", "skipped_excluded_client", "connectionId", invocation.ConnectionID, "method", invocation.Target)
			}
		} else {
			// Client is not connected to this instance, ignore.
			level.Debug(r.logger).Log("event", "received_client_message_for_remote_connection", "connectionId", invocation.ConnectionID)
		}
		return // Processed client message, exit
	}

	// 2. Message for a group (sent via InvokeGroup)
	if invocation.GroupName != "" && strings.HasPrefix(msg.Channel, redisGroupChannelPrefix) {
		groupName := invocation.GroupName // Group name is in the payload
		level.Debug(r.logger).Log("event", "processing_group_message", "group", groupName, "method", invocation.Target)

		// Iterate through local connections ONLY
		r.clients.Range(func(key, value interface{}) bool {
			connectionID := key.(string)
			hubConn := value.(signalr.HubConnection)

			// Check exclusion list first
			if _, excluded := excludedMap[connectionID]; excluded {
				level.Debug(r.logger).Log("event", "skipped_excluded_client_in_group", "connectionId", connectionID, "group", groupName, "method", invocation.Target)
				return true // Continue iterating
			}

			// Check if this local connection is actually in the target Redis group
			// This avoids sending to connections that might have been removed
			// from the group but haven't disconnected yet.
			isMember, err := r.redisClient.SIsMember(r.ctx, r.groupMembershipKey(groupName), connectionID).Result()
			if err != nil {
				level.Error(r.logger).Log("event", "redis_sismember_failed", "group", groupName, "connectionId", connectionID, "err", err)
				return true // Continue, maybe next check will succeed or Redis recovers
			}

			if isMember {
				// Send the invocation asynchronously
				go func(conn signalr.HubConnection, connID string) {
					// Timeout removed as per request
					err := conn.SendInvocation(uuid.NewString(), invocation.Target, invocation.Args)
					if err != nil {
						level.Error(r.logger).Log("event", "send_invocation_failed", "target", "group", "groupName", groupName, "connectionId", connID, "err", err)
					} else {
						level.Debug(r.logger).Log("event", "sent_invocation_local", "target", "group", "groupName", groupName, "connectionId", connID, "method", invocation.Target)
					}
				}(hubConn, connectionID)
			} else {
				// This can happen if the local cache (OnDisconnected cleanup) is slightly behind Redis,
				// or if the connection was removed from the group but not disconnected.
				level.Debug(r.logger).Log("event", "local_connection_not_in_redis_group", "group", groupName, "connectionId", connectionID)
			}

			return true // Continue iterating local clients
		})
		return // Processed group message, exit
	}

	// 3. Message for all clients (sent via InvokeAll)
	if invocation.ConnectionID == "" && invocation.GroupName == "" && msg.Channel == redisAllChannel {
		level.Debug(r.logger).Log("event", "processing_all_message", "method", invocation.Target)
		// Iterate through all local connections
		r.clients.Range(func(key, value interface{}) bool {
			connectionID := key.(string)
			hubConn := value.(signalr.HubConnection)

			// Check exclusion list
			if _, excluded := excludedMap[connectionID]; excluded {
				level.Debug(r.logger).Log("event", "skipped_excluded_client_in_all", "connectionId", connectionID, "method", invocation.Target)
				return true // Continue iterating
			}

			// Send the invocation asynchronously
			go func(conn signalr.HubConnection, connID string) {
				_, cancel := context.WithTimeout(r.ctx, 5*time.Second)
				defer cancel()
				err := conn.SendInvocation(uuid.NewString(), invocation.Target, invocation.Args)
				if err != nil {
					level.Error(r.logger).Log("event", "send_invocation_failed", "target", "all", "connectionId", connID, "err", err)
				} else {
					level.Debug(r.logger).Log("event", "sent_invocation_local", "target", "all", "connectionId", connID, "method", invocation.Target)
				}
			}(hubConn, connectionID)

			return true // Continue iterating local clients
		})
		return // Processed 'all' message, exit
	}

	// If none of the above matched, log it.
	level.Warn(r.logger).Log("event", "unhandled_redis_message", "channel", msg.Channel, "payload", msg.Payload)
}

// --- HubLifetimeManager Interface Implementation ---

func (r *redisHubLifetimeManager) OnConnected(conn signalr.HubConnection) {
	r.clients.Store(conn.ConnectionID(), conn)
	// Initialize local group cache for this connection
	r.connectionGroups.Store(conn.ConnectionID(), make(map[string]struct{}))
	level.Debug(r.logger).Log("event", "client_connected_local", "connectionId", conn.ConnectionID())
	// No Redis action needed on connect, only when adding to groups.
}

func (r *redisHubLifetimeManager) OnDisconnected(conn signalr.HubConnection) {
	connectionID := conn.ConnectionID()
	r.clients.Delete(connectionID)

	// Remove connection from all groups it belonged to in Redis
	// Use the local cache to know which groups to remove it from
	if groups, ok := r.connectionGroups.LoadAndDelete(connectionID); ok {
		groupSet, castOK := groups.(map[string]struct{})
		if !castOK {
			level.Error(r.logger).Log("event", "invalid_group_cache_type", "connectionId", connectionID)
			return // Cannot proceed if cache is corrupt
		}

		if len(groupSet) > 0 {
			// Use a pipeline for efficiency if removing from multiple groups
			pipe := r.redisClient.Pipeline()
			ctx, cancel := context.WithTimeout(r.ctx, 5*time.Second) // Timeout for Redis operation
			defer cancel()

			for groupName := range groupSet {
				redisKey := r.groupMembershipKey(groupName)
				pipe.SRem(ctx, redisKey, connectionID)
				level.Debug(r.logger).Log("event", "queueing_redis_srem", "group", groupName, "connectionId", connectionID)
			}

			_, err := pipe.Exec(ctx)
			if err != nil && err != redis.Nil { // redis.Nil might occur if pipeline was empty, ignore it
				level.Error(r.logger).Log("event", "redis_srem_pipeline_failed", "connectionId", connectionID, "err", err)
				// Log the error, but the connection is already considered disconnected locally.
			} else if err == nil {
				level.Debug(r.logger).Log("event", "executed_redis_srem_pipeline", "connectionId", connectionID, "group_count", len(groupSet))
			}
		}
	} else {
		// This might happen if OnDisconnected is called before OnConnected completes,
		// or if there was an issue storing the initial group set.
		level.Warn(r.logger).Log("event", "group_cache_not_found_on_disconnect", "connectionId", connectionID)
	}

	level.Debug(r.logger).Log("event", "client_disconnected_local", "connectionId", connectionID)
	// No need to publish a disconnect message typically. Other instances handle their own connections.
}

// InvokeAll publishes a message to the central 'all' channel.
// All server instances subscribed to this channel will process the message
// and send it to their *local* connections (unless excluded).
func (r *redisHubLifetimeManager) InvokeAll(target string, args []interface{}) {
	msg := redisInvocation{Target: target, Args: args}
	// ExcludedIDs can be added here if InvokeAllExcept is implemented
	payload, err := json.Marshal(msg)
	if err != nil {
		level.Error(r.logger).Log("event", "json_marshal_failed", "target", "all", "method", target, "err", err)
		return
	}
	err = r.redisClient.Publish(r.ctx, redisAllChannel, payload).Err()
	if err != nil {
		level.Error(r.logger).Log("event", "redis_publish_failed", "channel", redisAllChannel, "err", err)
	} else {
		level.Debug(r.logger).Log("event", "published_redis_message", "channel", redisAllChannel, "method", target)
	}
}

// InvokeClient publishes a message to the central 'all' channel, but includes
// the target ConnectionID in the payload.
// All instances receive the message, but only the instance managing that specific
// connection (if any) will actually send the message.
func (r *redisHubLifetimeManager) InvokeClient(connectionID string, target string, args []interface{}) {
	// Optimization: Check local first?
	// if client, ok := r.clients.Load(connectionID); ok {
	// 	 go func() { _ = client.(signalr.HubConnection).SendInvocation("", target, args) }()
	//   // If we send locally, should we still publish?
	//   // Publishing ensures the message is delivered even if the connection migrates *during* the send.
	//   // Let's stick to the Redis publish model for simplicity and consistency.
	// }

	msg := redisInvocation{Target: target, Args: args, ConnectionID: connectionID}
	payload, err := json.Marshal(msg)
	if err != nil {
		level.Error(r.logger).Log("event", "json_marshal_failed", "target", "client", "connectionId", connectionID, "method", target, "err", err)
		return
	}

	// Publish to the 'all' channel. handleRedisMessage will filter by ConnectionID.
	err = r.redisClient.Publish(r.ctx, redisAllChannel, payload).Err()
	if err != nil {
		level.Error(r.logger).Log("event", "redis_publish_failed", "channel", redisAllChannel, "targetConnection", connectionID, "err", err)
	} else {
		level.Debug(r.logger).Log("event", "published_redis_message", "channel", redisAllChannel, "target", "client", "connectionId", connectionID, "method", target)
	}
}

// InvokeGroup publishes a message to the group-specific channel.
// All server instances receive it and check their local connections
// against the Redis Set for that group before sending.
func (r *redisHubLifetimeManager) InvokeGroup(groupName string, target string, args []interface{}) {
	channel := r.groupChannel(groupName)
	msg := redisInvocation{Target: target, Args: args, GroupName: groupName}
	// ExcludedIDs can be added here if InvokeGroupExcept is implemented
	payload, err := json.Marshal(msg)
	if err != nil {
		level.Error(r.logger).Log("event", "json_marshal_failed", "target", "group", "groupName", groupName, "method", target, "err", err)
		return
	}

	err = r.redisClient.Publish(r.ctx, channel, payload).Err()
	if err != nil {
		level.Error(r.logger).Log("event", "redis_publish_failed", "channel", channel, "err", err)
	} else {
		level.Debug(r.logger).Log("event", "published_redis_message", "channel", channel, "method", target)
	}
}

// InvokeAllExcept publishes a message to the central 'all' channel, with excluded ConnectionIDs.
func (r *redisHubLifetimeManager) InvokeAllExcept(target string, args []interface{}, excludedIDs []string) {
	msg := redisInvocation{Target: target, Args: args, ExcludedIDs: excludedIDs}
	payload, err := json.Marshal(msg)
	if err != nil {
		level.Error(r.logger).Log("event", "json_marshal_failed", "target", "all_except", "method", target, "err", err)
		return
	}
	err = r.redisClient.Publish(r.ctx, redisAllChannel, payload).Err()
	if err != nil {
		level.Error(r.logger).Log("event", "redis_publish_failed", "channel", redisAllChannel, "err", err)
	} else {
		level.Debug(r.logger).Log("event", "published_redis_message", "channel", redisAllChannel, "target", "all_except", "method", target)
	}
}

// InvokeGroupExcept publishes a message to the group-specific channel, with excluded ConnectionIDs.
func (r *redisHubLifetimeManager) InvokeGroupExcept(groupName string, target string, args []interface{}, excludedIDs []string) {
	channel := r.groupChannel(groupName)
	msg := redisInvocation{Target: target, Args: args, GroupName: groupName, ExcludedIDs: excludedIDs}
	payload, err := json.Marshal(msg)
	if err != nil {
		level.Error(r.logger).Log("event", "json_marshal_failed", "target", "group_except", "groupName", groupName, "method", target, "err", err)
		return
	}

	err = r.redisClient.Publish(r.ctx, channel, payload).Err()
	if err != nil {
		level.Error(r.logger).Log("event", "redis_publish_failed", "channel", channel, "err", err)
	} else {
		level.Debug(r.logger).Log("event", "published_redis_message", "channel", channel, "target", "group_except", "groupName", groupName, "method", target)
	}
}

// AddToGroup adds the connection to the local cache and the Redis Set.
func (r *redisHubLifetimeManager) AddToGroup(groupName string, connectionID string) {
	// Add to local cache first
	groupsRaw, loaded := r.connectionGroups.Load(connectionID)
	if !loaded {
		// This might happen if OnConnected hasn't finished or connection disconnected rapidly.
		// We could try loading again or just log a warning.
		level.Warn(r.logger).Log("event", "group_cache_not_found_on_add", "connectionId", connectionID, "group", groupName)
		// Initialize it defensively?
		groupsRaw = make(map[string]struct{})
		r.connectionGroups.Store(connectionID, groupsRaw)
	}

	groupSet, castOK := groupsRaw.(map[string]struct{})
	if !castOK {
		level.Error(r.logger).Log("event", "invalid_group_cache_type_on_add", "connectionId", connectionID)
		// Don't proceed with Redis if local cache is bad.
		return
	}
	groupSet[groupName] = struct{}{} // Add to the local set

	// Add connection ID to the Redis Set for the group
	redisKey := r.groupMembershipKey(groupName)
	ctx, cancel := context.WithTimeout(r.ctx, 2*time.Second) // Timeout for Redis op
	defer cancel()
	err := r.redisClient.SAdd(ctx, redisKey, connectionID).Err()
	if err != nil {
		level.Error(r.logger).Log("event", "redis_sadd_failed", "group", groupName, "connectionId", connectionID, "err", err)
		// Rollback local cache add? Depends on consistency requirements.
		// For simplicity, we leave it in the local cache. If Redis fails,
		// the connection might not receive group messages, but disconnect
		// will still try to clean up the (potentially non-existent) Redis entry.
		delete(groupSet, groupName) // Rollback local add on Redis failure
	} else {
		level.Debug(r.logger).Log("event", "added_to_redis_group", "group", groupName, "connectionId", connectionID)
	}

	// No need to publish a message for AddToGroup.
}

// RemoveFromGroup removes the connection from the local cache and the Redis Set.
func (r *redisHubLifetimeManager) RemoveFromGroup(groupName string, connectionID string) {
	// Remove from local cache
	removedLocally := false
	if groupsRaw, ok := r.connectionGroups.Load(connectionID); ok {
		groupSet, castOK := groupsRaw.(map[string]struct{})
		if castOK {
			if _, exists := groupSet[groupName]; exists {
				delete(groupSet, groupName)
				removedLocally = true
			}
		} else {
			level.Error(r.logger).Log("event", "invalid_group_cache_type_on_remove", "connectionId", connectionID)
			// Don't attempt Redis removal if local cache is bad? Or proceed anyway?
			// Let's proceed with Redis removal attempt for robustness.
		}
	} else {
		level.Warn(r.logger).Log("event", "group_cache_not_found_on_remove", "connectionId", connectionID, "group", groupName)
	}

	// Remove connection ID from the Redis Set for the group
	redisKey := r.groupMembershipKey(groupName)
	ctx, cancel := context.WithTimeout(r.ctx, 2*time.Second) // Timeout for Redis op
	defer cancel()
	result, err := r.redisClient.SRem(ctx, redisKey, connectionID).Result()
	// Log error if SRem fails unexpectedly.
	// If SRem returns 0 (member didn't exist), it's not necessarily an error here,
	// especially if the local remove also failed or if there was inconsistency.
	if err != nil {
		level.Error(r.logger).Log("event", "redis_srem_failed", "group", groupName, "connectionId", connectionID, "err", err)
		// If Redis failed, should we re-add to local cache if we removed it?
		// For simplicity, leave local cache as removed.
	} else {
		// Log whether the member was actually removed from Redis (result > 0)
		level.Debug(r.logger).Log("event", "removed_from_redis_group", "group", groupName, "connectionId", connectionID, "redis_removed_count", result, "removed_locally", removedLocally)
	}

	// No need to publish a message for RemoveFromGroup.
}

// --- Helper methods ---

// groupChannel returns the Redis channel name for a specific group.
// Used for Pub/Sub.
func (r *redisHubLifetimeManager) groupChannel(groupName string) string {
	return redisGroupChannelPrefix + groupName
}

// clientChannel was considered but isn't used in the current implementation
// where client messages go via the 'all' channel.
// func (r *redisHubLifetimeManager) clientChannel(connectionID string) string {
// 	return redisClientChannelPrefix + connectionID
// }

// groupMembershipKey returns the Redis key for the Set storing members of a group.
func (r *redisHubLifetimeManager) groupMembershipKey(groupName string) string {
	return redisGroupMembershipPrefix + groupName
}

// Close stops the manager, closes the Redis PubSub, and closes the Redis client connection.
func (r *redisHubLifetimeManager) Close() error {
	level.Info(r.logger).Log("event", "closing_redis_manager")

	// 1. Signal the subscribeLoop goroutine to stop
	r.cancel()

	// 2. Close the PubSub connection (this might already happen in subscribeLoop on ctx.Done())
	//    It's generally safe to call Close multiple times on redis.PubSub
	if r.pubSub != nil {
		if err := r.pubSub.Close(); err != nil {
			level.Error(r.logger).Log("event", "redis_pubsub_close_error", "err", err)
			// Continue closing the main client anyway
		} else {
			level.Debug(r.logger).Log("event", "redis_pubsub_closed")
		}
	}

	// 3. Close the main Redis client connection
	err := r.redisClient.Close()
	if err != nil {
		level.Error(r.logger).Log("event", "redis_client_close_error", "err", err)
	} else {
		level.Info(r.logger).Log("event", "redis_client_closed")
	}

	// Clear local maps (optional, helps GC)
	r.clients = sync.Map{}
	r.connectionGroups = sync.Map{}

	return err // Return the error from closing the main client connection
}

// setProtocol sets the hub protocol on the Redis lifetime manager.
// This is called after the handshake determines the protocol.
// func (r *redisHubLifetimeManager) setProtocol(protocol hubProtocol) {
// 	r.protocol = protocol
// }
