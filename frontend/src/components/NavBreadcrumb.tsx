import { Breadcrumb, BreadcrumbItem, BreadcrumbLink } from '@chakra-ui/react'
import { useNavigate } from 'react-router-dom'
import type { ViewTreeNode } from '../types'

interface Props {
  diagram: ViewTreeNode
}

export default function NavBreadcrumb({ diagram }: Props) {
  const navigate = useNavigate()

  return (
    <Breadcrumb fontSize="sm" color="gray.500" sx={{ '& a': { color: 'gray.400', _hover: { color: 'gray.200' } } }}>
      <BreadcrumbItem>
        <BreadcrumbLink onClick={() => navigate('/views')} cursor="pointer">
          Diagrams
        </BreadcrumbLink>
      </BreadcrumbItem>
      <BreadcrumbItem isCurrentPage>
        <BreadcrumbLink>{diagram.name}</BreadcrumbLink>
      </BreadcrumbItem>
    </Breadcrumb>
  )
}
