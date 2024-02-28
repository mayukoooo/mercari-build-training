import { useState } from 'react'
import './App.css'
import { ItemList } from './components/ItemList'
import { Listing } from './components/Listing'

function App() {
  // reload ItemList after Listing complete
  const [reload, setReload] = useState(true)
  return (
    <div className="Root">
      <h1>Simple Mercari</h1>
      <Listing onListingCompleted={() => setReload(true)} />
      <div>
        <ItemList reload={reload} onLoadCompleted={() => setReload(false)} />
      </div>
    </div>
  )
}

export default App
