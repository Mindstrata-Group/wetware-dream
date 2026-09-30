'use client'


function Caret({ light }: { light?: boolean }) {
  return (
    <span style={{
      display: 'inline-block', width: 1.5, height: 13, marginLeft: 2, verticalAlign: '-2px',
      background: light ? '#fff' : '#0F6E56',
      animation: 'ms-blink 0.8s steps(2,end) infinite',
    }} />
  )
}

export { Caret }
