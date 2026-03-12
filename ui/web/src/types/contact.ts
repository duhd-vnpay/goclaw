export interface ChannelContact {
  id: string;
  channel_type: string;
  channel_instance?: string;
  sender_id: string;
  user_id?: string;
  display_name?: string;
  username?: string;
  avatar_url?: string;
  peer_kind?: string;
  merged_id?: string;
  first_seen_at: string;
  last_seen_at: string;
}
