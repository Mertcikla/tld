import { useEffect, useMemo, useState } from 'react'
import { useLocation } from 'react-router-dom'
import { Box, Button, Flex, Spinner, Text } from '@chakra-ui/react'
import type { ImpactScene } from '@buf/tldiagramcom_diagram.bufbuild_es/codeindex/v1/codeindex_pb.js'
import { mapImpactScene } from '../api/client'
import RepositoryChangeCanvas from '../components/RepositoryChangeCanvas'
import { ThemeProvider } from '../context/ThemeContext'
import { ConnectorStyleProvider } from '../context/ConnectorStyleContext'
import { isWailsApp, isWailsMac, isWailsWindows } from '../config/runtime'
import WindowsWindowControls from '../components/WindowsWindowControls'
import { decodeSceneFragment, localSceneURL } from '../utils/sceneUrl'
import { impactSceneMermaid } from '../utils/impactSceneMermaid'

export default function ComparisonScenePage() {
  const { hash } = useLocation()
  const [state, setState] = useState<{ hash: string; scene?: ImpactScene; error?: string }>()
  useEffect(() => {
    let active = true
    const urlLength = !isWailsApp && typeof window !== 'undefined' ? window.location.href.length : undefined
    void decodeSceneFragment(hash, isWailsApp, urlLength)
      .then((scene) => { if (active) setState({ hash, scene }) })
      .catch((error: unknown) => { if (active) setState({ hash, error: error instanceof Error ? error.message : 'Could not open this comparison.' }) })
    return () => { active = false }
  }, [hash])
  const current = state?.hash === hash ? state : undefined
  const embedded = useMemo(() => current?.scene ? {
    scene: mapImpactScene(current.scene),
    markdown: '```mermaid\n' + impactSceneMermaid(current.scene) + '```\n',
  } : undefined, [current?.scene])
  return (
    <ThemeProvider isAuthenticated={false}>
      <ConnectorStyleProvider>
        <Flex h="var(--app-viewport-height)" direction="column" bg="var(--bg-canvas)" overflow="hidden" data-testid="comparison-scene-page">
          <Flex px={4} py={2} align="center" gap={3} flexShrink={0} minH={isWailsApp ? '52px' : undefined} position="relative" pl={isWailsMac ? '90px' : 4} pr={isWailsWindows ? '138px' : 4} style={{ '--topbar-h': '52px', '--wails-draggable': isWailsApp ? 'drag' : 'no-drag' } as React.CSSProperties}>
            {isWailsWindows && <WindowsWindowControls />}
            <Text fontSize="sm" flex={1}>Comparison</Text>
            {embedded && !isWailsApp && <Button as="a" href={localSceneURL(hash)} size="sm">Open in TLD</Button>}
          </Flex>
          {!current ? <Box p={6}><Spinner aria-label="Loading comparison" /></Box>
            : current.error ? <Text p={6} role="alert" color="red.300">{current.error}</Text>
              : <>
                {!!embedded?.scene.warnings?.length && <Text px={4} color="orange.300" fontSize="sm">{embedded.scene.warnings.join(' ')}</Text>}
                {embedded?.scene.manifestComplete === false && <Text px={4} color="orange.300" fontSize="sm">The file manifest is incomplete; some changes may be missing.</Text>}
                <RepositoryChangeCanvas key={hash} diagram={null} selectedPath="" embedded={embedded} />
              </>}
        </Flex>
      </ConnectorStyleProvider>
    </ThemeProvider>
  )
}
