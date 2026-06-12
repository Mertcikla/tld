import { getSmoothStepPath, getStepPath, Position } from '@reactflow/core';

// Test same side aligned vertical connection
const resSmoothSame = getSmoothStepPath({
  sourceX: 300,
  sourceY: 100,
  sourcePosition: Position.Right,
  targetX: 300,
  targetY: 400,
  targetPosition: Position.Right,
  offset: 50,
});
console.log('Smooth same-side aligned:', resSmoothSame);

const resStepSame = getStepPath({
  sourceX: 300,
  sourceY: 100,
  sourcePosition: Position.Right,
  targetX: 300,
  targetY: 400,
  targetPosition: Position.Right,
  offset: 50,
});
console.log('Step same-side aligned:', resStepSame);

// Test opposite side aligned vertical connection (Right -> Left)
const resSmoothOpp = getSmoothStepPath({
  sourceX: 300,
  sourceY: 100,
  sourcePosition: Position.Right,
  targetX: 300,
  targetY: 400,
  targetPosition: Position.Left,
  offset: 50,
});
console.log('Smooth opposite-side aligned:', resSmoothOpp);
