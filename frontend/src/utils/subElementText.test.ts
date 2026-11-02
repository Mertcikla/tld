import { expect, it } from 'vitest'
import { expandCodeTabs, expandCodeTokens } from './subElementText'

it('uses four-column tab stops consistently across syntax tokens', () => {
  expect(expandCodeTabs('\tvalue\t=\t1')).toBe('    value   =   1')
  const tokens = [{ text: '\tvalue', color: 'red' }, { text: '\t=\t', color: 'white' }, { text: '1', color: 'blue' }]
  expect(expandCodeTokens(tokens).map(token => token.text).join('')).toBe(expandCodeTabs(tokens.map(token => token.text).join('')))
  expect(tokens[0].text).toBe('\tvalue')
})
