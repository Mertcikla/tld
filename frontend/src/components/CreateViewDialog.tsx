import { useRef } from 'react'
import {
  Button,
  FormControl,
  FormLabel,
  Input,
  Modal,
  ModalBody,
  ModalCloseButton,
  ModalContent,
  ModalFooter,
  ModalHeader,
  ModalOverlay,
  Text,
} from '@chakra-ui/react'

interface Props {
  isOpen: boolean
  onClose: () => void
  name: string
  onNameChange: (name: string) => void
  onSubmit: () => Promise<void>
  isSaving: boolean
  error?: string
  nameInputId?: string
  nameInputTestId?: string
  submitTestId?: string
}

// Share the creation form while each entry point owns its creation/navigation logic.
export default function CreateViewDialog({
  isOpen,
  onClose,
  name,
  onNameChange,
  onSubmit,
  isSaving,
  error,
  nameInputId = 'create-view-name',
  nameInputTestId,
  submitTestId,
}: Props) {
  const nameRef = useRef<HTMLInputElement>(null)
  const close = () => {
    if (!isSaving) onClose()
  }

  return (
    <Modal
      isOpen={isOpen}
      onClose={close}
      isCentered
      size="sm"
      initialFocusRef={nameRef}
      closeOnOverlayClick={!isSaving}
      closeOnEsc={!isSaving}
    >
      <ModalOverlay bg="blackAlpha.700" backdropFilter="blur(4px)" />
      <ModalContent
        mx={3}
        as="form"
        onSubmit={(event) => {
          event.preventDefault()
          if (!isSaving && name.trim()) void onSubmit()
        }}
      >
        <ModalHeader fontSize="md" fontWeight="semibold" py={4}>New view</ModalHeader>
        <ModalCloseButton isDisabled={isSaving} />
        <ModalBody py={4}>
          <FormControl isRequired>
            <FormLabel htmlFor={nameInputId} fontSize="sm">Name</FormLabel>
            <Input
              ref={nameRef}
              id={nameInputId}
              data-testid={nameInputTestId}
              name="name"
              size="sm"
              value={name}
              onChange={(event) => onNameChange(event.target.value)}
              isDisabled={isSaving}
              placeholder="My architecture"
            />
          </FormControl>
          {error && <Text role="alert" color="red.300" fontSize="sm" mt={3}>{error}</Text>}
        </ModalBody>
        <ModalFooter gap={2} py={3}>
          <Button type="button" variant="ghost" size="sm" onClick={close} isDisabled={isSaving}>Cancel</Button>
          <Button
            type="submit"
            data-testid={submitTestId}
            variant="primary"
            size="sm"
            px={5}
            isLoading={isSaving}
            isDisabled={!name.trim() || isSaving}
          >
            Create view
          </Button>
        </ModalFooter>
      </ModalContent>
    </Modal>
  )
}
