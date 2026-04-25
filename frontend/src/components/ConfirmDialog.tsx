import { useRef } from 'react'
import {
  AlertDialog,
  AlertDialogBody,
  AlertDialogContent,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogOverlay,
  Button,
} from '@chakra-ui/react'

interface Props {
  isOpen: boolean
  onClose: () => void
  onConfirm: () => void
  title: string
  body: string
  confirmLabel?: string
  confirmColorScheme?: string
  isLoading?: boolean
}

export default function ConfirmDialog({
  isOpen,
  onClose,
  onConfirm,
  title,
  body,
  confirmLabel = 'Delete',
  confirmColorScheme = 'red',
  isLoading,
}: Props) {
  const cancelRef = useRef<HTMLButtonElement>(null)

  return (
    <AlertDialog isOpen={isOpen} leastDestructiveRef={cancelRef} onClose={onClose} isCentered>
      <AlertDialogOverlay bg="blackAlpha.700" backdropFilter="blur(4px)" />
      <AlertDialogContent mx={4} data-testid="confirm-dialog">
        <AlertDialogHeader>
          {title}
        </AlertDialogHeader>
        <AlertDialogBody fontSize="sm">
          {body}
        </AlertDialogBody>
        <AlertDialogFooter gap={2} pt={4}>
          <Button
            ref={cancelRef}
            data-testid="confirm-dialog-cancel"
            onClick={onClose}
            variant="ghost"
            size="sm"
          >
            Cancel
          </Button>
          <Button
            data-testid="confirm-dialog-confirm"
            onClick={onConfirm}
            isLoading={isLoading}
            size="sm"
            variant={confirmColorScheme === 'red' ? 'destructive' : undefined}
          >
            {confirmLabel}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
