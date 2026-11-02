import { expect, it } from 'vitest'
import { protobufSymbols } from './protobufSymbols'

it('extracts nested protobuf declarations and RPC signatures without treating comments or strings as code', () => {
  const source = ['// message Fake {', 'message User {', '  string text = 1 [default = "}"];', '  message Profile { string id = 1; }', '  enum State { ACTIVE = 0; }', '}', 'service Users {', '  rpc Find (User) returns (User);', '}', '/* service Hidden {} */'].join('\n')
  expect(protobufSymbols(source)).toEqual([
    { name: 'User', type: 'message', qualifiedName: 'User', startLine: 2, endLine: 6 },
    { name: 'Profile', type: 'message', qualifiedName: 'User.Profile', startLine: 4, endLine: 4 },
    { name: 'State', type: 'enum', qualifiedName: 'User.State', startLine: 5, endLine: 5 },
    { name: 'Users', type: 'service', qualifiedName: 'Users', startLine: 7, endLine: 9 },
    { name: 'Find', type: 'rpc', qualifiedName: 'Users.Find', startLine: 8, endLine: 8 },
  ])
})
