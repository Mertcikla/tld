import { Avatar, AvatarGroup, Box, HStack, Tooltip } from '@chakra-ui/react'
import { hexToRgba } from '../constants/colors'
import type { RealtimeUserPresence } from '../platform/types'

const COLLABORATION_CURSOR_COLORS = ['#38bdf8', '#f97316', '#a78bfa', '#22c55e', '#f43f5e', '#eab308', '#14b8a6']

function collaborationColorForUser(userId: string) {
  let hash = 0
  for (let i = 0; i < userId.length; i += 1) {
    hash = (hash * 31 + userId.charCodeAt(i)) >>> 0
  }
  return COLLABORATION_CURSOR_COLORS[hash % COLLABORATION_CURSOR_COLORS.length]
}

export interface CollaborationProps {
  viewers: RealtimeUserPresence[]
  collaborators: RealtimeUserPresence[]
  followUserId?: string | null
  onAvatarClick?: (userId: string) => void
}

export default function TopMenuBarCollaboration({ collaboration }: { collaboration: CollaborationProps }) {
  if (collaboration.viewers.length <= 1) return null

  return (
    <HStack
      spacing={3}
      mr={4}
      sx={{
        '@container topbar (max-width: 720px)': {
          display: 'none !important',
        },
      }}
    >
      <AvatarGroup size="sm" max={6} spacing="-10px">
        {collaboration.collaborators.map((member) => {
          const memberColor = collaborationColorForUser(member.user_id)
          const isFollowing = collaboration.followUserId === member.user_id
          return (
            <Tooltip key={member.user_id} label={`${member.username}${member.online ? ' (Online)' : ' (Offline)'}`}>
              <Box position="relative" transition="all 0.2s ease" _hover={{ zIndex: 20, transform: 'scale(1.08)' }}>
                <Avatar
                  name={member.username}
                  size="sm"
                  bg={memberColor}
                  color="white"
                  opacity={member.online ? 1 : 0.55}
                  border="1px solid"
                  borderColor={memberColor}
                  boxShadow={isFollowing ? `0 0 15px ${hexToRgba(memberColor, 0.6)}` : '0 4px 10px rgba(0,0,0,0.3)'}
                  cursor={collaboration.onAvatarClick ? 'pointer' : 'default'}
                  transform={isFollowing ? 'scale(1.12)' : 'none'}
                  onClick={() => collaboration.onAvatarClick?.(member.user_id)}
                />
              </Box>
            </Tooltip>
          )
        })}
      </AvatarGroup>
    </HStack>
  )
}
