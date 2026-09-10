import { Button } from '@chakra-ui/react'
import { isWailsApp, isWailsMac, isWailsWindows, tldVersion, wailsPlatform } from '../config/runtime'

const ISSUES_NEW_URL = 'https://github.com/Mertcikla/tld/issues/new'

function buildFeedbackUrl(): string {
  const platform = wailsPlatform ?? (isWailsMac ? 'darwin' : isWailsWindows ? 'windows' : 'web')
  const body = [
    '',
    '',
    '---',
    `Version: ${tldVersion}`,
    `Platform: ${platform}${isWailsApp ? ' (desktop)' : ' (web)'}`,
  ].join('\n')

  const params = new URLSearchParams({ title: '[Feedback] ', body })
  return `${ISSUES_NEW_URL}?${params.toString()}`
}

export default function FeedbackButton() {
  return (
    <Button
      as="a"
      href={buildFeedbackUrl()}
      target="_blank"
      rel="noopener noreferrer"
      size="sm"
      variant="outline"
      borderRadius="lg"
      borderColor="whiteAlpha.200"
      color="gray.200"
      fontWeight="600"
      flexShrink={0}
      px={3}
      leftIcon={
        <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <path d="M21 11.5a8.38 8.38 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.38 8.38 0 0 1-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.38 8.38 0 0 1 3.8-.9h.5a8.48 8.48 0 0 1 8 8v.5z" />
        </svg>
      }
      _hover={{ bg: 'whiteAlpha.100', borderColor: 'whiteAlpha.300', color: 'white' }}
      _active={{ bg: 'whiteAlpha.200' }}
    >
      Feedback
    </Button>
  )
}
