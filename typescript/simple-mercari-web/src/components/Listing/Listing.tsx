import { FC, useState } from 'react'
import { InputForm } from '../InputForm'

const server = process.env.REACT_APP_API_URL || 'http://127.0.0.1:9000'

interface Prop {
  onListingCompleted?: () => void
}

type formDataType = {
  name: string
  category: string
  image: string | File
}

export const Listing: FC<Prop> = (props) => {
  const { onListingCompleted } = props
  const initialState = {
    name: '',
    category: '',
    image: '',
  }
  const [values, setValues] = useState<formDataType>(initialState)

  const onValueChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    setValues({
      ...values,
      [event.target.name]: event.target.value,
    })
  }
  const onFileChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    setValues({
      ...values,
      [event.target.name]: event.target.files![0],
    })
  }
  const onSubmit = (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const data = new FormData()
    data.append('name', values.name)
    data.append('category', values.category)
    data.append('image', values.image)

    fetch(server.concat('/items'), {
      method: 'POST',
      mode: 'cors',
      body: data,
    })
      .then((response) => {
        console.log('POST status:', response.statusText)
        onListingCompleted && onListingCompleted()
      })
      .catch((error) => {
        console.error('POST error:', error)
      })
  }
  return (
    <form onSubmit={onSubmit}>
      <div className="FormWrapper">
        <InputForm
          label="Name"
          name="name"
          id="name"
          placeholder="name"
          onChange={onValueChange}
          required
        />
        <InputForm
          label="Category"
          name="category"
          id="category"
          placeholder="category"
          onChange={onValueChange}
          required
        />
        <input
          className="InputImage"
          type="file"
          name="image"
          id="image"
          onChange={onFileChange}
          required
        />
      </div>
      <button type="submit">List this item</button>
    </form>
  )
}
